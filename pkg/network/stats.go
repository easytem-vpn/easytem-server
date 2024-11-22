package network

import (
	"fmt"
	"time"
)

func NewIPStats(aggregationInterval time.Duration) *IPStats {
	return &IPStats{
		stats:    make(map[string]*NetworkStats),
		interval: aggregationInterval,
	}
}

func (is *IPStats) updateStats(ip1 string, ip2 string, bytesTransferred uint64) {
	is.mutex.Lock()
	defer is.mutex.Unlock()

	// Create a normalized key (always put lower IP first)
	key := normalizeKey(ip1, ip2)

	if _, exists := is.stats[key]; !exists {
		is.stats[key] = &NetworkStats{
			IP1:         getFirstIP(ip1, ip2),
			IP2:         getSecondIP(ip1, ip2),
			LastUpdated: time.Now(),
		}
	}

	is.stats[key].Bytes += bytesTransferred
	is.stats[key].Packets++
	is.stats[key].LastUpdated = time.Now()
}

// Helper function to create consistent keys
func normalizeKey(ip1, ip2 string) string {
	if ip1 < ip2 {
		return fmt.Sprintf("%s-%s", ip1, ip2)
	}
	return fmt.Sprintf("%s-%s", ip2, ip1)
}

func getFirstIP(ip1, ip2 string) string {
	if ip1 < ip2 {
		return ip1
	}
	return ip2
}

func getSecondIP(ip1, ip2 string) string {
	if ip1 < ip2 {
		return ip2
	}
	return ip1
}

func (is *IPStats) GetStats() map[string]NetworkStats {
	is.mutex.RLock()
	defer is.mutex.RUnlock()

	result := make(map[string]NetworkStats)
	for key, stats := range is.stats {
		result[key] = NetworkStats{
			IP1:         stats.IP1,
			IP2:         stats.IP2,
			Bytes:       stats.Bytes,
			Packets:     stats.Packets,
			LastUpdated: stats.LastUpdated,
		}
	}
	return result
}

func (is *IPStats) cleanup() {
	is.mutex.Lock()
	defer is.mutex.Unlock()

	threshold := time.Now().Add(-is.interval)
	for ip, stats := range is.stats {
		if stats.LastUpdated.Before(threshold) {
			delete(is.stats, ip)
		}
	}
}
