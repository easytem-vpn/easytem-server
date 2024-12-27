package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"go-oauth/controller/api"
	"go-oauth/pkg/auth"
	"go-oauth/pkg/tasks"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var (
	authConfig  auth.Config
	wg          sync.WaitGroup
	JWT_SECRET  string
	JWT_EXPIRY  time.Duration
	taskManager *tasks.TaskManager
	rdb         *redis.Client
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

	// Update auth config
	authConfig = auth.Config{
		JWTSecret: JWT_SECRET,
		JWTExpiry: JWT_EXPIRY,
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// Routes
	r.POST("/social-login", api.HandleSocialLogin(authConfig))
	r.POST("/connectivity", api.HandleMobileInfo(rdb))
	r.GET("/del-connect", api.DelMobileInfo(rdb))

	protected := r.Group("/")
	protected.Use(auth.AuthMiddleware(authConfig))
	{
		//protected.GET("/protected-route", yourProtectedHandler)
	}

	// Initialize TaskManager
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI environment variable is required")
	}

	taskManager, err = tasks.NewTaskManager(mongoURI)
	if err != nil {
		log.Fatalf("Failed to initialize task manager: %v", err)
	}
	defer taskManager.Close()

	// Start the periodic tasks
	taskManager.StartPeriodicTasks(task1Interval)

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
