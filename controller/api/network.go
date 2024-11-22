package api

import (
	"go-oauth/pkg/dns"
	"go-oauth/pkg/network"
	"go-oauth/pkg/utils"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

type ReadableStats struct {
	IP1         string    `json:"ip1"`
	IP2         string    `json:"ip2"`
	Bytes       string    `json:"bytes"`
	Packets     uint64    `json:"packets"`
	LastUpdated time.Time `json:"last_updated"`
}

func HandleGetNetworkStats(monitor **network.Monitor, dnsCache *dns.Cache) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *monitor == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Monitoring not started"})
			return
		}

		stats := (*monitor).GetStats()
		readableStats := make([]ReadableStats, 0)

		var pairs []string
		for key := range stats {
			pairs = append(pairs, key)
		}
		sort.Strings(pairs)

		for _, pair := range pairs {
			stat := stats[pair]
			domain1 := dnsCache.GetDomainOrIP(stat.IP1)
			domain2 := dnsCache.GetDomainOrIP(stat.IP2)

			readableStats = append(readableStats, ReadableStats{
				IP1:         domain1,
				IP2:         domain2,
				Bytes:       utils.HumanizeBytes(stat.Bytes),
				Packets:     stat.Packets,
				LastUpdated: stat.LastUpdated,
			})
		}

		c.JSON(http.StatusOK, readableStats)
	}
}

func HandleStartMonitoring(monitor **network.Monitor) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Interface string `json:"interface"`
		}
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		*monitor = network.NewMonitor()
		if err := (*monitor).Start(req.Interface); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Monitoring started"})
	}
}

func HandleStopMonitoring(monitor **network.Monitor) gin.HandlerFunc {
	return func(c *gin.Context) {
		if *monitor != nil {
			(*monitor).Stop()
			*monitor = nil
		}
		c.JSON(http.StatusOK, gin.H{"message": "Monitoring stopped"})
	}
}
