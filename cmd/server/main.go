package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go-oauth/controller/api"
	"go-oauth/pkg/auth"
	"go-oauth/pkg/dns"
	"go-oauth/pkg/network"
	"go-oauth/pkg/tasks"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var (
	authConfig  auth.Config
	wg          sync.WaitGroup
	monitor     *network.Monitor
	JWT_SECRET  string
	JWT_EXPIRY  time.Duration
	redisClient *redis.Client
	dnsCache    *dns.Cache
	taskManager *tasks.TaskManager
)

func main() {
	r := gin.Default()

	err := godotenv.Load()
	if err != nil {
		log.Fatal(".env file failed to load!")
	}

	JWT_SECRET = os.Getenv("JWT_SECRET")

	expiryHours := os.Getenv("JWT_EXPIRY_HOURS")
	if expiryHours == "" {
		expiryHours = "168"
	}
	parsedHours, err := time.ParseDuration(expiryHours + "h")
	if err != nil {
		log.Fatal("Invalid JWT_EXPIRY_HOURS value")
	}
	JWT_EXPIRY = parsedHours

	if JWT_SECRET == "" {
		log.Fatal("Environment variables (JWT_SECRET) are required")
	}

	// Get intervals from environment variables
	task1Interval := getEnvDuration("TASK1_INTERVAL", "1m")
	task2Interval := getEnvDuration("TASK2_INTERVAL", "10s")

	// Update auth config
	authConfig = auth.Config{
		JWTSecret: JWT_SECRET,
		JWTExpiry: JWT_EXPIRY,
	}

	// Initialize the DNS cache BEFORE setting up routes
	dnsCache = dns.NewCache()

	// Initialize Redis client
	redisClient = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Routes
	r.POST("/social-login", api.HandleSocialLogin(authConfig))

	// Network routes
	r.POST("/start-monitoring", api.HandleStartMonitoring(&monitor))
	r.POST("/stop-monitoring", api.HandleStopMonitoring(&monitor))
	r.GET("/network-stats", api.HandleGetNetworkStats(&monitor, dnsCache))

	protected := r.Group("/")
	protected.Use(auth.AuthMiddleware(authConfig))
	{
		//protected.GET("/protected-route", yourProtectedHandler)
	}

	// Start network monitoring on default interface
	defaultInterface := os.Getenv("DEFAULT_NETWORK_INTERFACE")
	if defaultInterface == "" {
		defaultInterface = "eth0" // fallback to eth0 if not specified
	}

	monitor = network.NewMonitor()
	if err := monitor.Start(defaultInterface); err != nil {
		log.Printf("Warning: Failed to start initial network monitoring: %v", err)
	} else {
		log.Printf("Network monitoring started on interface: %s", defaultInterface)
	}

	// Test Redis connection
	ctx := context.Background()
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Printf("Warning: Redis connection failed: %v", err)
	}

	// Initialize TaskManager
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI environment variable is required")
	}

	taskManager, err = tasks.NewTaskManager(monitor, redisClient, dnsCache, mongoURI)
	if err != nil {
		log.Fatalf("Failed to initialize task manager: %v", err)
	}
	defer taskManager.Close()

	// Start the periodic tasks
	taskManager.StartPeriodicTasks(task1Interval, task2Interval)

	// Add this before starting the goroutine
	wg.Add(1)

	go func() {
		if err := r.Run(fmt.Sprintf(":%s", os.Getenv("PORT"))); err != nil {
			log.Printf("Server error: %v", err)
		}
		taskManager.Stop()
		wg.Done()
	}()

	// Wait for all goroutines to complete
	wg.Wait()
}

func getEnvDuration(key string, defaultValue string) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		value = defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("Invalid duration for %s, using default", key)
		duration, _ = time.ParseDuration(defaultValue)
	}
	return duration
}
