package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// SlidingWindow implements the Sliding Window rate limiting algorithm.
//
// How it works:
// 1. Maintains a window of fixed duration (e.g., 1 second)
// 2. Counts requests within the current window
// 3. Only allows new requests if count is below the limit
// 4. Automatically removes requests outside the window
// 5. More accurate than Fixed Window (no cliff problem)
//
// Advantages:
// - More accurate than fixed window (prevents traffic spikes at boundaries)
// - Smooth rate limiting without cliff behavior
// - Better for APIs with strict rate limits
//
// Disadvantages:
// - More memory intensive (stores individual request times)
// - Slightly higher computational overhead
// - Not ideal for very short intervals
//
// Configuration parameters:
// - maxRequests: Maximum number of requests allowed in the window
// - windowDuration: Duration of the sliding window (e.g., 1 second)
//
// Example: Allow 100 requests per second (strictly)
type SlidingWindow struct {
	store          storage.Store
	maxRequests    int64
	windowDuration time.Duration
	logger         *zap.Logger
}

// slidingWindowState represents the state of a sliding window rate limiter
type slidingWindowState struct {
	Timestamps []int64 `json:"timestamps"` // Unix nanosecond timestamps of requests
}

// NewSlidingWindow creates a new Sliding Window rate limiter.
//
// Parameters:
// - store: Backend storage (Redis, in-memory, etc.)
// - maxRequests: Maximum requests allowed in the window
// - windowDuration: Duration of the window (e.g., 1 second, 1 minute)
// - logger: Logger instance for debugging
//
// Example: Allow 100 requests per second
//
//	limiter := NewSlidingWindow(store, 100, time.Second, logger)
func NewSlidingWindow(store storage.Store, maxRequests int64, windowDuration time.Duration, logger *zap.Logger) *SlidingWindow {
	return &SlidingWindow{
		store:          store,
		maxRequests:    maxRequests,
		windowDuration: windowDuration,
		logger:         logger,
	}
}

// Allow checks if a request should be allowed under the sliding window algorithm.
func (sw *SlidingWindow) Allow(ctx context.Context, key string) (bool, error) {
	stateKey := sw.stateKey(key)
	now := time.Now().UnixNano()

	// Retrieve current state
	stateStr, err := sw.store.Get(ctx, stateKey)
	if err != nil {
		sw.logger.Error("failed to get sliding window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	var state slidingWindowState
	if stateStr != "" {
		if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			sw.logger.Warn("failed to parse sliding window state, reinitializing", zap.String("key", stateKey), zap.Error(err))
			state = slidingWindowState{Timestamps: []int64{}}
		}
	}

	// Remove timestamps outside the window
	windowStart := now - sw.windowDuration.Nanoseconds()
	filteredTimestamps := []int64{}
	for _, ts := range state.Timestamps {
		if ts > windowStart {
			filteredTimestamps = append(filteredTimestamps, ts)
		}
	}

	// Check if request is allowed
	allowed := int64(len(filteredTimestamps)) < sw.maxRequests

	if allowed {
		// Add current request timestamp
		filteredTimestamps = append(filteredTimestamps, now)
	}

	// Update state
	state.Timestamps = filteredTimestamps
	stateJSON, err := json.Marshal(state)
	if err != nil {
		sw.logger.Error("failed to marshal sliding window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	if err := sw.store.Set(ctx, stateKey, string(stateJSON), sw.windowDuration+1*time.Second); err != nil {
		sw.logger.Error("failed to set sliding window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	return allowed, nil
}

// Reset clears the sliding window state for a specific key.
func (sw *SlidingWindow) Reset(ctx context.Context, key string) error {
	stateKey := sw.stateKey(key)
	if err := sw.store.Delete(ctx, stateKey); err != nil {
		sw.logger.Error("failed to reset sliding window state", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("reset sliding window state: %w", err)
	}
	return nil
}

// Close performs cleanup when the rate limiter is no longer needed.
func (sw *SlidingWindow) Close() error {
	return nil
}

// stateKey generates a unique key for storing sliding window state
func (sw *SlidingWindow) stateKey(key string) string {
	return fmt.Sprintf("limiter:sliding_window:%s", key)
}
