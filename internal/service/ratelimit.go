package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// RateLimitConfig holds the configuration for a rate limiter
type RateLimitConfig struct {
	Algorithm      string `json:"algorithm"`       // token_bucket, sliding_window, fixed_window, leaky_bucket
	Limit          int64  `json:"limit"`           // max requests/tokens
	WindowSeconds  int    `json:"window_seconds"`  // time window in seconds
	RefillRate     int    `json:"refill_rate"`     // tokens per interval (for token bucket)
	RefillInterval int    `json:"refill_interval"` // interval in seconds (for token bucket)
}

// RateLimitService provides business logic for rate limiting
type RateLimitService struct {
	store    storage.Store
	limiters map[string]limiter.RateLimiter
	Logger   *zap.Logger
}

// NewRateLimitService creates a new rate limit service
func NewRateLimitService(store storage.Store, logger *zap.Logger) *RateLimitService {
	return &RateLimitService{
		store:    store,
		limiters: make(map[string]limiter.RateLimiter),
		Logger:   logger,
	}
}

// ValidateConfig validates the rate limit configuration
func (s *RateLimitService) ValidateConfig(cfg *RateLimitConfig) error {
	if cfg.Algorithm == "" {
		return errors.New("algorithm is required")
	}

	validAlgorithms := map[string]bool{
		"token_bucket":   true,
		"sliding_window": true,
		"fixed_window":   true,
		"leaky_bucket":   true,
	}

	if !validAlgorithms[cfg.Algorithm] {
		return fmt.Errorf("invalid algorithm: %s", cfg.Algorithm)
	}

	if cfg.Limit <= 0 {
		return errors.New("limit must be greater than 0")
	}

	if cfg.WindowSeconds <= 0 {
		return errors.New("window_seconds must be greater than 0")
	}

	if cfg.Algorithm == "token_bucket" {
		if cfg.RefillRate <= 0 {
			return errors.New("refill_rate must be greater than 0 for token bucket")
		}
		if cfg.RefillInterval <= 0 {
			return errors.New("refill_interval must be greater than 0 for token bucket")
		}
	}

	return nil
}

// GetConfig retrieves the configuration for a key
func (s *RateLimitService) GetConfig(ctx context.Context, key string) (*RateLimitConfig, error) {
	configKey := s.ConfigKey(key)
	configStr, err := s.store.Get(ctx, configKey)
	if err != nil {
		return nil, err
	}

	if configStr == "" {
		return nil, errors.New("configuration not found")
	}

	var config RateLimitConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return &config, nil
}

// GetMetadata retrieves the metadata for a key
func (s *RateLimitService) GetMetadata(ctx context.Context, key string) (map[string]interface{}, error) {
	metadataKey := s.MetadataKey(key)
	metadataStr, err := s.store.Get(ctx, metadataKey)
	if err != nil {
		return nil, err
	}

	if metadataStr == "" {
		return nil, errors.New("metadata not found")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return metadata, nil
}

// GetOrCreateLimiter gets an existing limiter or creates a new one
func (s *RateLimitService) GetOrCreateLimiter(limiterKey string, cfg *RateLimitConfig) (limiter.RateLimiter, error) {
	if lim, ok := s.limiters[limiterKey]; ok {
		return lim, nil
	}

	windowDuration := time.Duration(cfg.WindowSeconds) * time.Second
	leakRate := cfg.Limit / int64(cfg.WindowSeconds)
	if leakRate == 0 {
		leakRate = 1
	}

	var lim limiter.RateLimiter
	switch cfg.Algorithm {
	case "token_bucket":
		refillInterval := time.Duration(cfg.RefillInterval) * time.Second
		lim = limiter.NewTokenBucket(s.store, float64(cfg.RefillRate), refillInterval, float64(cfg.Limit), s.Logger)

	case "sliding_window":
		lim = limiter.NewSlidingWindow(s.store, cfg.Limit, windowDuration, s.Logger)

	case "fixed_window":
		lim = limiter.NewFixedWindow(s.store, cfg.Limit, windowDuration, s.Logger)

	case "leaky_bucket":
		lim = limiter.NewLeakyBucket(s.store, cfg.Limit, leakRate, windowDuration, s.Logger)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", cfg.Algorithm)
	}

	s.limiters[limiterKey] = lim
	return lim, nil
}

// CalculateRemaining calculates the remaining requests/tokens
func (s *RateLimitService) CalculateRemaining(ctx context.Context, key string, cfg *RateLimitConfig, allowed bool) int64 {
	// Simplified calculation based on algorithm
	// This is a best-effort estimate since actual remaining depends on algorithm internals
	remaining := cfg.Limit
	if !allowed {
		remaining = 0
	}
	return remaining
}

// CalculateResetAt calculates the reset timestamp
func (s *RateLimitService) CalculateResetAt(cfg *RateLimitConfig) int64 {
	// Window-based algorithms reset at the start of the next window
	now := time.Now()
	windowsSinceEpoch := now.Unix() / int64(cfg.WindowSeconds)
	nextWindowStart := (windowsSinceEpoch + 1) * int64(cfg.WindowSeconds)
	return nextWindowStart
}

// CalculateRetryAfter calculates seconds to wait before retry
func (s *RateLimitService) CalculateRetryAfter(cfg *RateLimitConfig) int {
	// Calculate seconds until next window
	now := time.Now()
	windowsSinceEpoch := now.Unix() / int64(cfg.WindowSeconds)
	nextWindowStart := time.Unix((windowsSinceEpoch+1)*int64(cfg.WindowSeconds), 0)
	retryAfter := int(nextWindowStart.Sub(now).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter
}

// ConfigKey generates a key for storing configuration
func (s *RateLimitService) ConfigKey(key string) string {
	return fmt.Sprintf("ratelimit:config:%s", key)
}

// MetadataKey generates a key for storing metadata
func (s *RateLimitService) MetadataKey(key string) string {
	return fmt.Sprintf("ratelimit:metadata:%s", key)
}

// LimiterKey generates a key for storing limiter instance
func (s *RateLimitService) LimiterKey(key, algorithm string) string {
	return fmt.Sprintf("%s:%s", key, algorithm)
}

// SetConfig stores the rate limit configuration
func (s *RateLimitService) SetConfig(ctx context.Context, key string, config *RateLimitConfig) error {
	if err := s.ValidateConfig(config); err != nil {
		return err
	}

	configKey := s.ConfigKey(key)
	configJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	now := time.Now()
	metadata := map[string]interface{}{
		"config":     config,
		"created_at": now.Unix(),
		"updated_at": now.Unix(),
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	// Store both config and metadata
	if err := s.store.Set(ctx, configKey, string(configJSON), 0); err != nil {
		return err
	}

	metadataKey := s.MetadataKey(key)
	if err := s.store.Set(ctx, metadataKey, string(metadataJSON), 0); err != nil {
		return err
	}

	return nil
}

// CheckLimit checks if a request is allowed
func (s *RateLimitService) CheckLimit(ctx context.Context, key string) (allowed bool, remaining int64, resetAt int64, retryAfter int, err error) {
	// Get configuration
	config, err := s.GetConfig(ctx, key)
	if err != nil {
		return false, 0, 0, 0, err
	}

	// Get or create limiter
	limiterKey := s.LimiterKey(key, config.Algorithm)
	rateLimiter, err := s.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		return false, 0, 0, 0, err
	}

	// Check if request is allowed
	allowed, err = rateLimiter.Allow(ctx, key)
	if err != nil {
		// Fail open on error
		allowed = true
	}

	// Calculate metrics for response
	remaining = s.CalculateRemaining(ctx, key, config, allowed)
	resetAt = s.CalculateResetAt(config)
	retryAfter = s.CalculateRetryAfter(config)

	return allowed, remaining, resetAt, retryAfter, nil
}

// ResetLimit resets a rate limit configuration and clears its state
func (s *RateLimitService) ResetLimit(ctx context.Context, key string) error {
	// Get configuration to find limiter type
	config, err := s.GetConfig(ctx, key)
	if err != nil {
		return err
	}

	// Get or create limiter and reset it
	limiterKey := s.LimiterKey(key, config.Algorithm)
	rateLimiter, err := s.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		return err
	}

	if err := rateLimiter.Reset(ctx, key); err != nil {
		return err
	}

	// Delete configuration and metadata
	configKey := s.ConfigKey(key)
	metadataKey := s.MetadataKey(key)

	_ = s.store.Delete(ctx, configKey)
	_ = s.store.Delete(ctx, metadataKey)

	return nil
}

// GetStatus retrieves the current status of a rate limit
func (s *RateLimitService) GetStatus(ctx context.Context, key string) (config *RateLimitConfig, createdAt int64, updatedAt int64, nextReset int64, err error) {
	// Get configuration
	config, err = s.GetConfig(ctx, key)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	// Get metadata
	metadata, err := s.GetMetadata(ctx, key)
	if err != nil {
		metadata = map[string]interface{}{
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
		}
	}

	createdAt = int64(0)
	updatedAt = int64(0)
	if ca, ok := metadata["created_at"].(float64); ok {
		createdAt = int64(ca)
	}
	if ua, ok := metadata["updated_at"].(float64); ok {
		updatedAt = int64(ua)
	}

	nextReset = s.CalculateResetAt(config)

	return config, createdAt, updatedAt, nextReset, nil
}
