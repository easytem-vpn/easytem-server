package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"go-oauth/pkg/dns"
	"go-oauth/pkg/network"
	"go-oauth/pkg/utils"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TaskManager struct {
	monitor     *network.Monitor
	redisClient *redis.Client
	dnsCache    *dns.Cache
	stopChan    chan struct{}
	mongoClient *mongo.Client
}

func NewTaskManager(monitor *network.Monitor, redisClient *redis.Client, dnsCache *dns.Cache, mongoURI string) (*TaskManager, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	return &TaskManager{
		monitor:     monitor,
		redisClient: redisClient,
		dnsCache:    dnsCache,
		stopChan:    make(chan struct{}),
		mongoClient: mongoClient,
	}, nil
}

func (tm *TaskManager) StartPeriodicTasks(task1Interval, task2Interval time.Duration) {
	go tm.runPeriodicTasks(task1Interval, task2Interval)
}

func (tm *TaskManager) Stop() {
	close(tm.stopChan)
}

func (tm *TaskManager) runPeriodicTasks(task1Interval, task2Interval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in periodic tasks: %v", r)
			// Restart the periodic tasks
			go tm.runPeriodicTasks(task1Interval, task2Interval)
		}
	}()

	task1Ticker := time.NewTicker(task1Interval)
	task2Ticker := time.NewTicker(task2Interval)

	defer func() {
		task1Ticker.Stop()
		task2Ticker.Stop()
	}()

	for {
		select {
		case <-tm.stopChan:
			return
		case <-task1Ticker.C:
			if err := tm.sendingStatsToMongo(); err != nil {
				log.Printf("Error in sendingStatsToMongo: %v", err)
			}
		case <-task2Ticker.C:
			if err := tm.fetchingNetworkStats(); err != nil {
				log.Printf("Error in fetchingNetworkStats: %v", err)
			}
		}
	}
}

func (tm *TaskManager) sendingStatsToMongo() error {
	ctx := context.Background()

	keys, err := tm.redisClient.Keys(ctx, "network_stats:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get keys from Redis: %v", err)
	}

	if len(keys) == 0 {
		log.Println("No statistics found in Redis")
		return nil
	}

	collection := tm.mongoClient.Database("network_stats").Collection("traffic")

	for _, key := range keys {
		jsonData, err := tm.redisClient.Get(ctx, key).Result()
		if err != nil {
			log.Printf("Error getting data for key %s: %v", key, err)
			continue
		}

		var statsEntry map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &statsEntry); err != nil {
			log.Printf("Error unmarshaling data for key %s: %v", key, err)
			continue
		}

		statsEntry["_id"] = key
		statsEntry["recorded_at"] = time.Now()

		opts := options.Update().SetUpsert(true)
		filter := bson.M{"_id": key}
		update := bson.M{"$set": statsEntry}

		_, err = collection.UpdateOne(ctx, filter, update, opts)
		if err != nil {
			log.Printf("Error upserting document for key %s: %v", key, err)
			continue
		}

		log.Printf("Successfully transferred stats for %s to MongoDB", key)
	}

	return nil
}

func (tm *TaskManager) fetchingNetworkStats() error {
	log.Println("Fetching Network Statistics...")

	if tm.monitor == nil {
		return fmt.Errorf("monitor not initialized")
	}

	stats := tm.monitor.GetStats()
	log.Println("Current Network Statistics:")

	var pairs []string
	for key := range stats {
		pairs = append(pairs, key)
	}
	sort.Strings(pairs)

	ctx := context.Background()

	for _, pair := range pairs {
		stat := stats[pair]
		ip1HostName := tm.dnsCache.GetDomainOrIP(stat.IP1)
		ip2HostName := tm.dnsCache.GetDomainOrIP(stat.IP2)

		statsEntry := map[string]interface{}{
			"ip1":          ip1HostName,
			"ip2":          ip2HostName,
			"bytes":        utils.HumanizeBytes(stat.Bytes),
			"packets":      stat.Packets,
			"last_updated": stat.LastUpdated,
			"timestamp":    time.Now(),
		}

		statsKey := fmt.Sprintf("network_stats:%s:%s", ip1HostName, ip2HostName)

		statsJSON, err := json.Marshal(statsEntry)
		if err != nil {
			log.Printf("Error marshaling stats for %s: %v", statsKey, err)
			continue
		}

		err = tm.redisClient.Set(ctx, statsKey, string(statsJSON), 7*24*time.Hour).Err()
		if err != nil {
			log.Printf("Error saving stats to Redis for %s: %v", statsKey, err)
			continue
		}

		log.Printf("Connection between %s and %s:\n\tBytes: %s\n\tPackets: %d\n\tLast Updated: %v",
			ip1HostName,
			ip2HostName,
			utils.HumanizeBytes(stat.Bytes),
			stat.Packets,
			stat.LastUpdated,
		)
	}

	return nil
}

func (tm *TaskManager) Close() error {
	if tm.mongoClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tm.mongoClient.Disconnect(ctx); err != nil {
			return fmt.Errorf("failed to disconnect MongoDB client: %v", err)
		}
	}
	return nil
}
