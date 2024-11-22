package dns

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	cache map[string]string
	mu    sync.RWMutex
}

func NewCache() *Cache {
	return &Cache{
		cache: make(map[string]string),
	}
}

func (c *Cache) GetDomainOrIP(ip string) string {
	// Check cache first
	c.mu.RLock()
	if domain, exists := c.cache[ip]; exists {
		c.mu.RUnlock()
		return domain
	}
	c.mu.RUnlock()

	// Do lookup with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var result string
	done := make(chan bool)

	go func() {
		names, err := net.LookupAddr(ip)
		if err != nil || len(names) == 0 {
			result = ip
		} else {
			result = strings.TrimSuffix(names[0], ".")
		}
		done <- true
	}()

	select {
	case <-ctx.Done():
		result = ip // Timeout occurred
	case <-done:
		// Lookup completed
	}

	// Cache the result
	c.mu.Lock()
	c.cache[ip] = result
	c.mu.Unlock()

	return result
}
