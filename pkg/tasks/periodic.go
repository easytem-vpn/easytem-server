package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/icholy/digest"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ctx = context.Background()

type TaskManager struct {
	stopChan     chan struct{}
	mongoClient  *mongo.Client
	username     string
	password     string
	uri          string
	dnsStats     map[string]*DNSStats
	lastStopTime int64
	lastID       string
	redisClient  *redis.Client
}

type Session struct {
	FirstPacket  int64       `json:"firstPacket"`
	TotDataBytes int         `json:"totDataBytes"`
	IPProtocol   int         `json:"ipProtocol"`
	Node         string      `json:"node"`
	LastPacket   int64       `json:"lastPacket"`
	Source       Endpoint    `json:"source"`
	Destination  Endpoint    `json:"destination"`
	Client       Traffic     `json:"client"`
	Server       Traffic     `json:"server"`
	Network      NetworkInfo `json:"network"`
	DNS          DNSInfo     `json:"dns"`
	ID           string      `json:"id"`
}

type Endpoint struct {
	AS      map[string]interface{} `json:"as"`
	Geo     map[string]interface{} `json:"geo"`
	Packets int                    `json:"packets"`
	Port    int                    `json:"port"`
	IP      string                 `json:"ip"`
	Bytes   int                    `json:"bytes"`
}

type Traffic struct {
	Bytes int `json:"bytes"`
}

type NetworkInfo struct {
	Packets int `json:"packets"`
	Bytes   int `json:"bytes"`
}

type DNSInfo struct {
	Host []string `json:"host"`
}

type IPStats struct {
	TotalBytes int
	User       string
}

type DNSStats struct {
	DNS string
}

type DomainStats struct {
	Domain     string `json:"domain"`
	TotalBytes int64  `json:"total_bytes"`
	User       string `json:"user"`
	NetType    string `json:"net_type"`
}

func NewTaskManager(mongoURI string) (*TaskManager, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	return &TaskManager{
		stopChan:     make(chan struct{}),
		mongoClient:  mongoClient,
		username:     os.Getenv("ARKIME_USERNAME"),
		password:     os.Getenv("ARKIME_PASSWORD"),
		uri:          os.Getenv("ARKIME_URI"),
		dnsStats:     make(map[string]*DNSStats),
		lastStopTime: time.Now().Unix() - 300,
		lastID:       "",
		redisClient:  redisClient,
	}, nil
}

func (tm *TaskManager) StartPeriodicTasks(task1Interval time.Duration) {
	go tm.runPeriodicTasks(task1Interval)
}

func (tm *TaskManager) Stop() {
	close(tm.stopChan)
}

func (tm *TaskManager) runPeriodicTasks(task1Interval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in periodic tasks: %v", r)
			// Restart the periodic tasks
			go tm.runPeriodicTasks(task1Interval)
		}
	}()

	task1Ticker := time.NewTicker(task1Interval)

	defer func() {
		task1Ticker.Stop()
	}()

	for {
		select {
		case <-tm.stopChan:
			return
		case <-task1Ticker.C:
			if err := tm.sendingStatsToMongo(); err != nil {
				log.Printf("Error in sendingStatsToMongo: %v", err)
			}
		}
	}
}

func (tm *TaskManager) sendingStatsToMongo() error {

	currentTime := time.Now().Unix()
	startTime := tm.lastStopTime
	tm.lastStopTime = currentTime

	t := &digest.Transport{
		Username: tm.username,
		Password: tm.password,
	}

	url := fmt.Sprintf("%s/sessions.json?order=firstPacket:desc&startTime=%d&stopTime=%d&facets=1&length=2000000",
		tm.uri, startTime, currentTime)

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		log.Fatalln(err)
		return nil
	}

	resp, err := t.RoundTrip(req)
	if err != nil {
		log.Fatalln(err)
		return nil
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %v", err)
	}

	dataBytes, _ := json.Marshal(result["data"])

	var sessions []Session
	if err := json.Unmarshal(dataBytes, &sessions); err != nil {
		return fmt.Errorf("failed to unmarshal sessions: %v", err)
	}

	aggregatedStats := make(map[string]*IPStats)

	for _, session := range sessions {
		if session.ID == tm.lastID && tm.lastID != "" {
			break
		}

		if len(session.DNS.Host) > 0 {
			req, _ := http.NewRequest("GET", tm.uri+"/api/session/localhost/"+session.ID+"/detail", nil)
			resp, _ := t.RoundTrip(req)
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			re := regexp.MustCompile(`expr="ip.dns" value="\b(?:\d{1,3}\.){3}\d{1,3}\b`)
			ips := re.FindAllString(string(body), -1)

			if len(ips) > 0 {
				for _, ip := range ips {
					ipAddr := ip[21:]
					if _, exists := tm.dnsStats[ipAddr]; !exists {
						tm.dnsStats[ipAddr] = &DNSStats{
							DNS: session.DNS.Host[0],
						}
					}
				}
			}
		} else if session.TotDataBytes > 0 {
			destIP := session.Destination.IP
			if _, exists := aggregatedStats[destIP]; !exists {
				aggregatedStats[destIP] = &IPStats{
					User: session.Source.IP,
				}
			}

			stats := aggregatedStats[destIP]
			stats.TotalBytes += session.TotDataBytes
		}
	}

	tm.lastID = sessions[0].ID

	collection := tm.mongoClient.Database("network_stats").Collection("traffic_stats")
	ctx := context.Background()

	for _, stats := range tm.dnsStats {
		log.Println(stats.DNS)
	}

	log.Println("DNS Found:", len(tm.dnsStats))
	log.Println("IP Found:", len(aggregatedStats))
	log.Println("--------------------------------")

	for destIP, stats := range aggregatedStats {
		var domain = destIP
		if dnsInfo, exists := tm.dnsStats[destIP]; exists {
			domain = dnsInfo.DNS
		}

		// First, try to find existing document
		filter := bson.M{"domain": domain, "user": stats.User}

		netype, value_err := tm.redisClient.Get(ctx, stats.User).Result()

		if value_err == redis.Nil {
			netype = "wifi"
		}

		var existingStats DomainStats
		err := collection.FindOne(ctx, filter).Decode(&existingStats)

		if err == mongo.ErrNoDocuments {
			// Document doesn't exist, create new one
			newStats := DomainStats{
				Domain:     domain,
				TotalBytes: int64(stats.TotalBytes),
				User:       stats.User,
				NetType:    netype,
			}
			_, err = collection.InsertOne(ctx, newStats)
		} else if err == nil {
			update := bson.M{
				"$set": bson.M{
					"totalbytes": existingStats.TotalBytes + int64(stats.TotalBytes),
				},
			}
			_, err = collection.UpdateOne(ctx, filter, update)
		}

		if err != nil {
			return fmt.Errorf("failed to update domain stats: %v", err)
		}
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
