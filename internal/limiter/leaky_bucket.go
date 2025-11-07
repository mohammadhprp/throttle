package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// LeakyBucket implements the Leaky Bucket rate limiting algorithm.
//
// How it works:
// 1. Requests are added to a bucket (queue) when they arrive
// 2. Requests are processed (leak out) at a fixed rate
// 3. If the bucket is full, new requests are rejected
// 4. Smooths traffic by processing at constant rate
// 5. Useful for protecting backend services from burst traffic
//
// Advantages:
// - Smooths out burst traffic
// - Constant processing rate is predictable
// - Good for protecting backend services
// - Prevents queue buildup
//
// Disadvantages:
// - Can reject requests even if bucket not full (leaky while processing)
// - More complex than token bucket
// - May not handle bursty traffic well
//
// Configuration parameters:
// - bucketCapacity: Maximum number of requests the bucket can hold
// - leakRate: Number of requests to process per leak interval
// - leakInterval: Duration between leak operations
//
// Example: Bucket holds 100 requests, leak 10 per second
type LeakyBucket struct {
	store           storage.Store
	bucketCapacity  int64
	leakRate        int64
	leakInterval    time.Duration
	logger          *zap.Logger
}

// leakyBucketState represents the state of a leaky bucket rate limiter
type leakyBucketState struct {
	Count         int64 `json:"count"`          // Current number of requests in bucket
	LastLeakTime  int64 `json:"last_leak_time"` // When tokens were last leaked (Unix nanoseconds)
}

// NewLeakyBucket creates a new Leaky Bucket rate limiter.
//
// Parameters:
// - store: Backend storage (Redis, in-memory, etc.)
// - bucketCapacity: Maximum capacity of the bucket
// - leakRate: Number of requests to leak per interval
// - leakInterval: Duration between leak operations
// - logger: Logger instance for debugging
//
// Example: Bucket capacity 100, leak 10 per second
//
//	limiter := NewLeakyBucket(store, 100, 10, time.Second, logger)
func NewLeakyBucket(store storage.Store, bucketCapacity int64, leakRate int64, leakInterval time.Duration, logger *zap.Logger) *LeakyBucket {
	return &LeakyBucket{
		store:          store,
		bucketCapacity: bucketCapacity,
		leakRate:       leakRate,
		leakInterval:   leakInterval,
		logger:         logger,
	}
}

// Allow checks if a request should be allowed under the leaky bucket algorithm.
func (lb *LeakyBucket) Allow(ctx context.Context, key string) (bool, error) {
	stateKey := lb.stateKey(key)
	now := time.Now().UnixNano()

	// Retrieve current state
	stateStr, err := lb.store.Get(ctx, stateKey)
	if err != nil {
		lb.logger.Error("failed to get leaky bucket state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	var state leakyBucketState
	if stateStr != "" {
		if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			lb.logger.Warn("failed to parse leaky bucket state, reinitializing", zap.String("key", stateKey), zap.Error(err))
			state = leakyBucketState{Count: 0, LastLeakTime: now}
		}
	} else {
		// Initialize new state
		state = leakyBucketState{
			Count:        0,
			LastLeakTime: now,
		}
	}

	// Calculate how many requests to leak based on elapsed time
	elapsed := now - state.LastLeakTime
	intervalsElapsed := elapsed / lb.leakInterval.Nanoseconds()
	tokensToLeak := int64(intervalsElapsed) * lb.leakRate

	// Leak tokens
	state.Count -= tokensToLeak
	if state.Count < 0 {
		state.Count = 0
	}

	// Update last leak time
	state.LastLeakTime = now

	// Check if request can be added to bucket
	allowed := state.Count < lb.bucketCapacity

	if allowed {
		state.Count++
	}

	// Update state
	stateJSON, err := json.Marshal(state)
	if err != nil {
		lb.logger.Error("failed to marshal leaky bucket state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	// Store with long expiration
	if err := lb.store.Set(ctx, stateKey, string(stateJSON), 24*time.Hour); err != nil {
		lb.logger.Error("failed to set leaky bucket state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	return allowed, nil
}

// Reset clears the leaky bucket state for a specific key.
func (lb *LeakyBucket) Reset(ctx context.Context, key string) error {
	stateKey := lb.stateKey(key)
	if err := lb.store.Delete(ctx, stateKey); err != nil {
		lb.logger.Error("failed to reset leaky bucket state", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("reset leaky bucket state: %w", err)
	}
	return nil
}

// Close performs cleanup when the rate limiter is no longer needed.
func (lb *LeakyBucket) Close() error {
	return nil
}

// stateKey generates a unique key for storing leaky bucket state
func (lb *LeakyBucket) stateKey(key string) string {
	return fmt.Sprintf("limiter:leaky_bucket:%s", key)
}
