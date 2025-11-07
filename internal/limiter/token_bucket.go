package limiter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// TokenBucket implements the Token Bucket rate limiting algorithm.
//
// How it works:
// 1. A bucket is filled with tokens at a fixed rate (refill_rate tokens per refill_interval)
// 2. Each request consumes 1 token
// 3. If the bucket is empty, the request is rejected
// 4. The bucket has a maximum capacity (max_tokens)
// 5. Unused tokens are lost (they don't accumulate beyond max_tokens)
//
// Advantages:
// - Handles burst traffic well (up to max_tokens)
// - Smooth rate limiting with predictable behavior
// - Can accommodate varying request sizes
// - Works well for APIs with bursty traffic patterns
//
// Disadvantages:
// - Requires persistent state for distributed systems
// - Clock skew can cause issues in distributed environments
//
// Configuration parameters:
// - refill_rate: Number of tokens added per refill_interval
// - refill_interval: How frequently tokens are added
// - max_tokens: Maximum tokens the bucket can hold (capacity)
type TokenBucket struct {
	store          storage.Store
	refillRate     float64       // tokens per refill_interval
	refillInterval time.Duration // how often to refill
	maxTokens      float64       // maximum capacity
	logger         *zap.Logger
}

// tokenBucketState represents the persistent state of a token bucket
type tokenBucketState struct {
	Tokens       float64 `json:"tokens"`         // current token count
	LastFillTime int64   `json:"last_fill_time"` // when tokens were last refilled (Unix nanoseconds)
}

// NewTokenBucket creates a new Token Bucket rate limiter.
//
// Parameters:
// - store: Backend storage (Redis, in-memory, etc.)
// - refillRate: How many tokens to add per refill_interval
// - refillInterval: How frequently to refill tokens
// - maxTokens: Maximum capacity of the bucket (also the initial token count)
// - logger: Logger instance for debugging
//
// Example: Allow 10 requests per second with bursts up to 20 requests
//
//	limiter := NewTokenBucket(store, 10, time.Second, 20, logger)
func NewTokenBucket(store storage.Store, refillRate float64, refillInterval time.Duration, maxTokens float64, logger *zap.Logger) *TokenBucket {
	return &TokenBucket{
		store:          store,
		refillRate:     refillRate,
		refillInterval: refillInterval,
		maxTokens:      maxTokens,
		logger:         logger,
	}
}

// Allow checks if a request should be allowed under the token bucket algorithm.
// It returns true if a token was successfully consumed, false otherwise.
func (tb *TokenBucket) Allow(ctx context.Context, key string) (bool, error) {
	stateKey := tb.stateKey(key)

	// Retrieve or initialize bucket state
	state, err := tb.getBucketState(ctx, stateKey)
	if err != nil {
		tb.logger.Error("failed to get bucket state", zap.String("key", key), zap.Error(err))
		// Fail open: allow the request if we can't check state
		return true, err
	}

	// Calculate tokens to add based on elapsed time
	now := time.Now()
	lastFillTime := time.Unix(0, state.LastFillTime)
	elapsed := now.Sub(lastFillTime)
	tokensToAdd := tb.refillRate * (elapsed.Seconds() / tb.refillInterval.Seconds())

	// Update token count, capped at max_tokens
	state.Tokens = min(state.Tokens+tokensToAdd, tb.maxTokens)
	state.LastFillTime = now.UnixNano()

	// Check if we can consume a token
	allowed := state.Tokens >= 1.0

	if allowed {
		state.Tokens -= 1.0
	}

	// Persist the updated state
	if err := tb.setBucketState(ctx, stateKey, state); err != nil {
		tb.logger.Error("failed to set bucket state", zap.String("key", key), zap.Error(err))
		// Fail open: allow the request if we can't persist state
		return true, err
	}

	return allowed, nil
}

// Reset clears the bucket state for a specific key.
func (tb *TokenBucket) Reset(ctx context.Context, key string) error {
	stateKey := tb.stateKey(key)
	if err := tb.store.Delete(ctx, stateKey); err != nil {
		tb.logger.Error("failed to reset bucket state", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("reset bucket state: %w", err)
	}
	return nil
}

// Close performs cleanup when the rate limiter is no longer needed.
func (tb *TokenBucket) Close() error {
	return nil
}

// getBucketState retrieves the bucket state from storage or initializes it
func (tb *TokenBucket) getBucketState(ctx context.Context, stateKey string) (*tokenBucketState, error) {
	// Try to retrieve existing state from storage
	stateStr, err := tb.store.Get(ctx, stateKey)
	if err != nil {
		// If we get an error other than key not found, return it
		return nil, fmt.Errorf("get bucket state: %w", err)
	}

	// If the key doesn't exist (empty string), initialize with full bucket
	if stateStr == "" {
		return &tokenBucketState{
			Tokens:       tb.maxTokens,
			LastFillTime: time.Now().UnixNano(),
		}, nil
	}

	// Parse the stored state
	var state tokenBucketState
	if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
		// If we can't parse, reinitialize
		tb.logger.Warn("failed to parse bucket state, reinitializing", zap.String("key", stateKey), zap.Error(err))
		return &tokenBucketState{
			Tokens:       tb.maxTokens,
			LastFillTime: time.Now().UnixNano(),
		}, nil
	}

	return &state, nil
}

// setBucketState persists the bucket state to storage
func (tb *TokenBucket) setBucketState(ctx context.Context, stateKey string, state *tokenBucketState) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal bucket state: %w", err)
	}

	// Use a longer expiration to prevent stale state cleanup
	// State will expire after 24 hours of inactivity
	expiration := 24 * time.Hour
	if err := tb.store.Set(ctx, stateKey, string(stateJSON), expiration); err != nil {
		return fmt.Errorf("set bucket state: %w", err)
	}

	return nil
}

// stateKey generates a unique key for storing bucket state
func (tb *TokenBucket) stateKey(key string) string {
	return fmt.Sprintf("limiter:token_bucket:%s", key)
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
