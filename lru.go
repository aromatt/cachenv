package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

// CacheEntry represents a single cache entry
type CacheEntry struct {
	Hash       string
	StdoutPath string
	StderrPath string
}

// LRUCache is a simple LRU cache for managing cache entries
type LRUCache struct {
	Capacity int
	Entries  map[string]*CacheEntry
	Order    []string
}

// NewLRUCache creates a new LRU cache with the given capacity
func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		Capacity: capacity,
		Entries:  make(map[string]*CacheEntry),
		Order:    make([]string, 0, capacity),
	}
}

// Add adds a new entry to the cache or updates an existing one
func (c *LRUCache) Add(hash string, entry *CacheEntry) {
	if _, exists := c.Entries[hash]; !exists {
		// If adding this entry exceeds the capacity, evict the least recently used one
		if len(c.Order) >= c.Capacity {
			oldest := c.Order[0]
			c.Order = c.Order[1:]
			delete(c.Entries, oldest)
			// Additionally, delete the associated files
			os.Remove(c.Entries[oldest].StdoutPath)
			os.Remove(c.Entries[oldest].StderrPath)
		}
		c.Order = append(c.Order, hash)
	} else {
		// Move to the end to mark as recently used
		for i, ordHash := range c.Order {
			if ordHash == hash {
				c.Order = append(c.Order[:i], c.Order[i+1:]...)
				c.Order = append(c.Order, hash)
				break
			}
		}
	}
	c.Entries[hash] = entry
}

// Exists checks if an entry exists in the cache
func (c *LRUCache) Exists(hash string) bool {
	_, exists := c.Entries[hash]
	return exists
}

// GenerateHash generates a SHA-256 hash for the given command and arguments
func GenerateHash(command string, args []string) string {
	concatCmd := command + " " + strings.Join(args, " ")
	h := sha256.Sum256([]byte(concatCmd))
	return fmt.Sprintf("%x", h[:])
}
