package service

import (
	"testing"

	"github.com/mohammadhprp/throttle/internal/service"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// setupTest creates and returns a logger, store, and rate limit service for testing
// It automatically handles logger syncing via cleanup
func setupTest(t *testing.T) (store storage.Store, svc *service.RateLimitService, cleanup func()) {
	store = storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()

	cleanup = func() {
		_ = logger.Sync()
	}

	svc = service.NewRateLimitService(store, logger)
	return store, svc, cleanup
}

// createTokenBucketConfig returns a valid token bucket configuration
func createTokenBucketConfig() *service.RateLimitConfig {
	return &service.RateLimitConfig{
		Algorithm:      service.AlgorithmTokenBucket,
		Limit:          100,
		WindowSeconds:  10,
		RefillRate:     10,
		RefillInterval: 1,
	}
}

// createFixedWindowConfig returns a valid fixed window configuration
func createFixedWindowConfig() *service.RateLimitConfig {
	return &service.RateLimitConfig{
		Algorithm:     service.AlgorithmFixedWindow,
		Limit:         50,
		WindowSeconds: 60,
	}
}

// createSlidingWindowConfig returns a valid sliding window configuration
func createSlidingWindowConfig() *service.RateLimitConfig {
	return &service.RateLimitConfig{
		Algorithm:     service.AlgorithmSlidingWindow,
		Limit:         100,
		WindowSeconds: 30,
	}
}

// createLeakyBucketConfig returns a valid leaky bucket configuration
func createLeakyBucketConfig() *service.RateLimitConfig {
	return &service.RateLimitConfig{
		Algorithm:     service.AlgorithmLeakyBucket,
		Limit:         100,
		WindowSeconds: 10,
	}
}
