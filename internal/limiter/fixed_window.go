package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// FixedWindow implements the Fixed Window (Counting) rate limiting algorithm.
//
// How it works:
// 1. Divides time into fixed windows (e.g., 1-second buckets)
// 2. Counts requests in the current window
// 3. Resets counter at the start of each new window
// 4. Allows requests if count is below limit
// 5. Simple and fast to implement
//
// Advantages:
// - Very simple to understand and implement
// - Minimal memory usage
// - Fast computation
// - Good for simple rate limiting
//
// Disadvantages:
// - Allows traffic spikes at window boundaries (2x limit possible)
// - Less accurate than sliding window
// - Can have cliff behavior where requests suddenly drop at boundaries
//
// Configuration parameters:
// - maxRequests: Maximum number of requests allowed per window
// - windowDuration: Duration of each window (e.g., 1 second, 1 minute)
//
// Example: Allow 100 requests per second
type FixedWindow struct {
	store          storage.Store
	maxRequests    int64
	windowDuration time.Duration
	logger         *zap.Logger
}

// fixedWindowState represents the state of a fixed window rate limiter
type fixedWindowState struct {
	Count     int64 `json:"count"`      // Number of requests in current window
	WindowEnd int64 `json:"window_end"` // Unix nanosecond timestamp when window ends
}

// NewFixedWindow creates a new Fixed Window rate limiter.
//
// Parameters:
// - store: Backend storage (Redis, in-memory, etc.)
// - maxRequests: Maximum requests allowed per window
// - windowDuration: Duration of each window (e.g., 1 second, 1 minute)
// - logger: Logger instance for debugging
//
// Example: Allow 100 requests per second
//
//	limiter := NewFixedWindow(store, 100, time.Second, logger)
func NewFixedWindow(store storage.Store, maxRequests int64, windowDuration time.Duration, logger *zap.Logger) *FixedWindow {
	return &FixedWindow{
		store:          store,
		maxRequests:    maxRequests,
		windowDuration: windowDuration,
		logger:         logger,
	}
}

// Allow checks if a request should be allowed under the fixed window algorithm.
func (fw *FixedWindow) Allow(ctx context.Context, key string) (bool, error) {
	stateKey := fw.stateKey(key)
	now := time.Now().UnixNano()

	// Retrieve current state
	stateStr, err := fw.store.Get(ctx, stateKey)
	if err != nil {
		fw.logger.Error("failed to get fixed window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	var state fixedWindowState
	windowEnd := now + fw.windowDuration.Nanoseconds()

	if stateStr != "" {
		if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
			fw.logger.Warn("failed to parse fixed window state, reinitializing", zap.String("key", stateKey), zap.Error(err))
			state = fixedWindowState{Count: 0, WindowEnd: windowEnd}
		}

		// Check if we're in a new window
		if now >= state.WindowEnd {
			// New window started
			state.Count = 0
			state.WindowEnd = now + fw.windowDuration.Nanoseconds()
		}
	} else {
		// Initialize new state
		state = fixedWindowState{
			Count:     0,
			WindowEnd: windowEnd,
		}
	}

	// Check if request is allowed
	allowed := state.Count < fw.maxRequests

	if allowed {
		state.Count++
	}

	// Update state
	stateJSON, err := json.Marshal(state)
	if err != nil {
		fw.logger.Error("failed to marshal fixed window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	// Set expiration to window end + some buffer
	expiration := time.Until(time.Unix(0, state.WindowEnd)) + 1*time.Second
	if expiration < 0 {
		expiration = fw.windowDuration + 1*time.Second
	}

	if err := fw.store.Set(ctx, stateKey, string(stateJSON), expiration); err != nil {
		fw.logger.Error("failed to set fixed window state", zap.String("key", key), zap.Error(err))
		return true, err // Fail open
	}

	return allowed, nil
}

// Reset clears the fixed window state for a specific key.
func (fw *FixedWindow) Reset(ctx context.Context, key string) error {
	stateKey := fw.stateKey(key)
	if err := fw.store.Delete(ctx, stateKey); err != nil {
		fw.logger.Error("failed to reset fixed window state", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("reset fixed window state: %w", err)
	}
	return nil
}

// Close performs cleanup when the rate limiter is no longer needed.
func (fw *FixedWindow) Close() error {
	return nil
}

// stateKey generates a unique key for storing fixed window state
func (fw *FixedWindow) stateKey(key string) string {
	return fmt.Sprintf("limiter:fixed_window:%s", key)
}
