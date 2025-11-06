package storage

import (
	"context"
	"time"
)

// Store defines the interface for rate limiter storage backends
type Store interface {
	// Increment increments the counter for the given key
	Increment(ctx context.Context, key string, expiration time.Duration) (int64, error)

	// IncrementBy increments the counter by a specific amount
	IncrementBy(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error)

	// Get retrieves the current value for the given key
	Get(ctx context.Context, key string) (int64, error)

	// Set sets the value for the given key with expiration
	Set(ctx context.Context, key string, value int64, expiration time.Duration) error

	// Delete removes the key from storage
	Delete(ctx context.Context, key string) error

	// ZAdd adds a member with score to a sorted set
	ZAdd(ctx context.Context, key string, score float64, member string, expiration time.Duration) error

	// ZRemRangeByScore removes members with scores in the given range
	ZRemRangeByScore(ctx context.Context, key string, min, max float64) error

	// ZCount counts members with scores in the given range
	ZCount(ctx context.Context, key string, min, max float64) (int64, error)

	// Expire sets expiration for a key
	Expire(ctx context.Context, key string, expiration time.Duration) error

	// Ping checks if the storage is accessible
	Ping(ctx context.Context) error

	// Close closes the storage connection
	Close() error
}
