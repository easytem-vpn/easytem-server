package network

import (
	"sync"
	"time"
)

// NetworkStats holds network statistics
type NetworkStats struct {
	IP1         string // First IP in the pair
	IP2         string // Second IP in the pair
	Bytes       uint64 // Total bytes transferred between IPs
	Packets     uint64 // Total packets transferred between IPs
	LastUpdated time.Time
}

// IPStats holds IP statistics
type IPStats struct {
	stats    map[string]*NetworkStats // key will be normalized "ip1-ip2"
	mutex    sync.RWMutex
	interval time.Duration
}
