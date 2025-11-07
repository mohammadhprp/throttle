package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore implements Store interface using in-memory storage
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string]*StorageValue
	sortedSets map[string]*SortedSet
	stopChan chan struct{}
}

// StorageValue represents a value with expiration
type StorageValue struct {
	value      string
	expiration time.Time
}

// SortedSet represents a sorted set data structure
type SortedSet struct {
	members map[string]float64 // member -> score
	expiration time.Time
}

// ZSetMember represents a member in a sorted set
type ZSetMember struct {
	Score  float64
	Member string
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	ms := &MemoryStore{
		data:       make(map[string]*StorageValue),
		sortedSets: make(map[string]*SortedSet),
		stopChan:   make(chan struct{}),
	}

	// Start a goroutine to clean up expired keys
	go ms.cleanupExpiredKeys()

	return ms
}

// cleanupExpiredKeys periodically removes expired keys
func (ms *MemoryStore) cleanupExpiredKeys() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.removeExpiredKeys()
		case <-ms.stopChan:
			return
		}
	}
}

// removeExpiredKeys removes all expired keys from storage
func (ms *MemoryStore) removeExpiredKeys() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()

	// Clean up expired regular values
	for key, val := range ms.data {
		if !val.expiration.IsZero() && val.expiration.Before(now) {
			delete(ms.data, key)
		}
	}

	// Clean up expired sorted sets
	for key, zset := range ms.sortedSets {
		if !zset.expiration.IsZero() && zset.expiration.Before(now) {
			delete(ms.sortedSets, key)
		}
	}
}

// Increment increments the counter for the given key
func (ms *MemoryStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	val, exists := ms.data[key]
	var newVal int64 = 1

	if exists {
		// Parse existing value
		if _, err := fmt.Sscanf(val.value, "%d", &newVal); err != nil {
			newVal = 1
		} else {
			newVal++
		}
	}

	// Set the new value with expiration
	expirationTime := time.Now().Add(expiration)
	ms.data[key] = &StorageValue{
		value:      fmt.Sprintf("%d", newVal),
		expiration: expirationTime,
	}

	return newVal, nil
}

// IncrementBy increments the counter by a specific amount
func (ms *MemoryStore) IncrementBy(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	val, exists := ms.data[key]
	var currentVal int64 = 0

	if exists {
		// Parse existing value
		if _, err := fmt.Sscanf(val.value, "%d", &currentVal); err != nil {
			currentVal = 0
		}
	}

	newVal := currentVal + value

	// Set the new value with expiration
	expirationTime := time.Now().Add(expiration)
	ms.data[key] = &StorageValue{
		value:      fmt.Sprintf("%d", newVal),
		expiration: expirationTime,
	}

	return newVal, nil
}

// Get retrieves the current value for the given key
func (ms *MemoryStore) Get(ctx context.Context, key string) (string, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	val, exists := ms.data[key]
	if !exists {
		return "", nil
	}

	// Check if expired
	if !val.expiration.IsZero() && val.expiration.Before(time.Now()) {
		return "", nil
	}

	return val.value, nil
}

// Set sets the value for the given key with expiration
func (ms *MemoryStore) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	expirationTime := time.Now().Add(expiration)
	ms.data[key] = &StorageValue{
		value:      value,
		expiration: expirationTime,
	}

	return nil
}

// Delete removes the key from storage
func (ms *MemoryStore) Delete(ctx context.Context, key string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	delete(ms.data, key)
	delete(ms.sortedSets, key)

	return nil
}

// ZAdd adds a member with score to a sorted set
func (ms *MemoryStore) ZAdd(ctx context.Context, key string, score float64, member string, expiration time.Duration) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	zset, exists := ms.sortedSets[key]
	if !exists {
		zset = &SortedSet{
			members: make(map[string]float64),
		}
		ms.sortedSets[key] = zset
	}

	zset.members[member] = score
	zset.expiration = time.Now().Add(expiration)

	return nil
}

// ZRemRangeByScore removes members with scores in the given range
func (ms *MemoryStore) ZRemRangeByScore(ctx context.Context, key string, min, max float64) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	zset, exists := ms.sortedSets[key]
	if !exists {
		return nil
	}

	// Remove members within the score range
	for member, score := range zset.members {
		if score >= min && score <= max {
			delete(zset.members, member)
		}
	}

	// If the sorted set is empty, delete it
	if len(zset.members) == 0 {
		delete(ms.sortedSets, key)
	}

	return nil
}

// ZCount counts members with scores in the given range
func (ms *MemoryStore) ZCount(ctx context.Context, key string, min, max float64) (int64, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	zset, exists := ms.sortedSets[key]
	if !exists {
		return 0, nil
	}

	// Check if expired
	if !zset.expiration.IsZero() && zset.expiration.Before(time.Now()) {
		return 0, nil
	}

	count := int64(0)
	for _, score := range zset.members {
		if score >= min && score <= max {
			count++
		}
	}

	return count, nil
}

// Expire sets expiration for a key
func (ms *MemoryStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Handle regular values
	if val, exists := ms.data[key]; exists {
		val.expiration = time.Now().Add(expiration)
	}

	// Handle sorted sets
	if zset, exists := ms.sortedSets[key]; exists {
		zset.expiration = time.Now().Add(expiration)
	}

	return nil
}

// Ping checks if the storage is accessible
func (ms *MemoryStore) Ping(ctx context.Context) error {
	// In-memory storage is always accessible
	return nil
}

// Close closes the storage connection
func (ms *MemoryStore) Close() error {
	close(ms.stopChan)
	return nil
}
