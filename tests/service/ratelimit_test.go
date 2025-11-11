package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/service"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// TestRateLimitService_ValidateConfig_ValidTokenBucket tests valid token bucket configuration
func TestRateLimitService_ValidateConfig_ValidTokenBucket(t *testing.T) {
	_, svc, cleanup := setupTest(t)
	defer cleanup()

	config := createTokenBucketConfig()

	err := svc.ValidateConfig(config)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestRateLimitService_ValidateConfig_ValidFixedWindow tests valid fixed window configuration
func TestRateLimitService_ValidateConfig_ValidFixedWindow(t *testing.T) {
	_, svc, cleanup := setupTest(t)
	defer cleanup()

	config := createFixedWindowConfig()

	err := svc.ValidateConfig(config)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestRateLimitService_ValidateConfig_ValidSlidingWindow tests valid sliding window configuration
func TestRateLimitService_ValidateConfig_ValidSlidingWindow(t *testing.T) {
	_, svc, cleanup := setupTest(t)
	defer cleanup()

	config := createSlidingWindowConfig()

	err := svc.ValidateConfig(config)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestRateLimitService_ValidateConfig_ValidLeakyBucket tests valid leaky bucket configuration
func TestRateLimitService_ValidateConfig_ValidLeakyBucket(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "leaky_bucket",
		Limit:         100,
		WindowSeconds: 60,
	}

	err := svc.ValidateConfig(config)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestRateLimitService_ValidateConfig_MissingAlgorithm tests validation with missing algorithm
func TestRateLimitService_ValidateConfig_MissingAlgorithm(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "",
		Limit:         100,
		WindowSeconds: 10,
	}

	err := svc.ValidateConfig(config)
	if err == nil {
		t.Errorf("expected error for missing algorithm, got nil")
	}
}

// TestRateLimitService_ValidateConfig_InvalidAlgorithm tests validation with invalid algorithm
func TestRateLimitService_ValidateConfig_InvalidAlgorithm(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "invalid_algo",
		Limit:         100,
		WindowSeconds: 10,
	}

	err := svc.ValidateConfig(config)
	if err == nil {
		t.Errorf("expected error for invalid algorithm, got nil")
	}
}

// TestRateLimitService_ValidateConfig_ZeroLimit tests validation with zero limit
func TestRateLimitService_ValidateConfig_ZeroLimit(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         0,
		WindowSeconds: 10,
	}

	err := svc.ValidateConfig(config)
	if err == nil {
		t.Errorf("expected error for zero limit, got nil")
	}
}

// TestRateLimitService_ValidateConfig_ZeroWindow tests validation with zero window
func TestRateLimitService_ValidateConfig_ZeroWindow(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 0,
	}

	err := svc.ValidateConfig(config)
	if err == nil {
		t.Errorf("expected error for zero window, got nil")
	}
}

// TestRateLimitService_ValidateConfig_TokenBucketMissingRefillRate tests token bucket without refill rate
func TestRateLimitService_ValidateConfig_TokenBucketMissingRefillRate(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:      "token_bucket",
		Limit:          100,
		WindowSeconds:  10,
		RefillRate:     0,
		RefillInterval: 1,
	}

	err := svc.ValidateConfig(config)
	if err == nil {
		t.Errorf("expected error for missing refill rate, got nil")
	}
}

// TestRateLimitService_ConfigKey tests config key generation
func TestRateLimitService_ConfigKey(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	key := svc.ConfigKey("test_key")
	expected := "ratelimit:config:test_key"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

// TestRateLimitService_MetadataKey tests metadata key generation
func TestRateLimitService_MetadataKey(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	key := svc.MetadataKey("test_key")
	expected := "ratelimit:metadata:test_key"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

// TestRateLimitService_LimiterKey tests limiter key generation
func TestRateLimitService_LimiterKey(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	key := svc.LimiterKey("test_key", "fixed_window")
	expected := "test_key:fixed_window"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

// TestRateLimitService_CalculateRemaining tests remaining calculation
func TestRateLimitService_CalculateRemaining(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	// When allowed
	remaining := svc.CalculateRemaining(context.Background(), "test_key", config, true)
	if remaining != 100 {
		t.Errorf("expected remaining 100 when allowed, got %d", remaining)
	}

	// When not allowed
	remaining = svc.CalculateRemaining(context.Background(), "test_key", config, false)
	if remaining != 0 {
		t.Errorf("expected remaining 0 when not allowed, got %d", remaining)
	}
}

// TestRateLimitService_CalculateResetAt tests reset time calculation
func TestRateLimitService_CalculateResetAt(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	resetAt := svc.CalculateResetAt(config)

	// Reset time should be in the future
	now := time.Now().Unix()
	if resetAt <= now {
		t.Errorf("expected reset time in the future, got %d (now: %d)", resetAt, now)
	}
}

// TestRateLimitService_CalculateRetryAfter tests retry after calculation
func TestRateLimitService_CalculateRetryAfter(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	retryAfter := svc.CalculateRetryAfter(config)

	// Retry after should be at least 1 second
	if retryAfter < 1 {
		t.Errorf("expected retry after >= 1, got %d", retryAfter)
	}

	// Retry after should be <= window seconds
	if retryAfter > config.WindowSeconds {
		t.Errorf("expected retry after <= %d, got %d", config.WindowSeconds, retryAfter)
	}
}

// TestRateLimitService_GetConfig_NotFound tests GetConfig for non-existent key
func TestRateLimitService_GetConfig_NotFound(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	_, err := svc.GetConfig(context.Background(), "non_existent_key")
	if err == nil {
		t.Errorf("expected error for non-existent key, got nil")
	}
}

// TestRateLimitService_CheckLimit_ConfigNotFound tests CheckLimit with non-existent config
func TestRateLimitService_CheckLimit_ConfigNotFound(t *testing.T) {
	_, svc, cleanup := setupTest(t)
	defer cleanup()

	resp, err := svc.CheckLimit(context.Background(), "non_existent")

	if err == nil {
		t.Errorf("expected error for non-existent config, got nil")
	}

	if resp != nil {
		if resp.Allowed {
			t.Errorf("expected allowed=false for non-existent config, got true")
		}

		if resp.Remaining != 0 {
			t.Errorf("expected remaining=0, got %d", resp.Remaining)
		}
	}
}

// TestRateLimitService_ResetLimit_NotFound tests reset with non-existent key
func TestRateLimitService_ResetLimit_NotFound(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	err := svc.ResetLimit(context.Background(), "non_existent")
	if err == nil {
		t.Errorf("expected error for non-existent key, got nil")
	}
}

// TestRateLimitService_GetStatus_NotFound tests GetStatus for non-existent key
func TestRateLimitService_GetStatus_NotFound(t *testing.T) {
	_, svc, cleanup := setupTest(t)
	defer cleanup()

	_, err := svc.GetStatus(context.Background(), "non_existent")
	if err == nil {
		t.Errorf("expected error for non-existent key, got nil")
	}
}

// TestRateLimitService_NewRateLimitService tests proper initialization
func TestRateLimitService_NewRateLimitService(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	if svc == nil {
		t.Errorf("expected non-nil service")
	}

	if svc.Logger == nil {
		t.Errorf("expected non-nil logger")
	}
}


// TestRateLimitService_GetOrCreateLimiter tests limiter creation
func TestRateLimitService_GetOrCreateLimiter(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	limiterKey := svc.LimiterKey("test_key", config.Algorithm)

	// Create limiter first time
	limiter1, err := svc.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		t.Errorf("failed to create limiter: %v", err)
	}

	if limiter1 == nil {
		t.Errorf("expected non-nil limiter")
	}

	// Get limiter second time (should be cached)
	limiter2, err := svc.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		t.Errorf("failed to get cached limiter: %v", err)
	}

	// Verify it's the same instance (cached)
	if limiter1 != limiter2 {
		t.Errorf("expected same limiter instance from cache")
	}
}

// TestRateLimitService_ConfigMarshaling tests configuration JSON marshaling
func TestRateLimitService_ConfigMarshaling(t *testing.T) {
	config := &service.RateLimitConfig{
		Algorithm:      "token_bucket",
		Limit:          100,
		WindowSeconds:  10,
		RefillRate:     10,
		RefillInterval: 1,
	}

	// Marshal
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Errorf("failed to marshal config: %v", err)
	}

	// Unmarshal
	var retrievedConfig service.RateLimitConfig
	err = json.Unmarshal(configJSON, &retrievedConfig)
	if err != nil {
		t.Errorf("failed to unmarshal config: %v", err)
	}

	if retrievedConfig.Algorithm != config.Algorithm {
		t.Errorf("algorithm mismatch after marshaling")
	}

	if retrievedConfig.Limit != config.Limit {
		t.Errorf("limit mismatch after marshaling")
	}

	if retrievedConfig.RefillRate != config.RefillRate {
		t.Errorf("refill rate mismatch after marshaling")
	}
}

// TestRateLimitService_InvalidAlgorithmForGetOrCreateLimiter tests invalid algorithm
func TestRateLimitService_InvalidAlgorithmForGetOrCreateLimiter(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewRateLimitService(store, logger)

	config := &service.RateLimitConfig{
		Algorithm:     "invalid_algo",
		Limit:         100,
		WindowSeconds: 60,
	}

	limiterKey := svc.LimiterKey("test_key", config.Algorithm)

	_, err := svc.GetOrCreateLimiter(limiterKey, config)
	if err == nil {
		t.Errorf("expected error for invalid algorithm, got nil")
	}
}
