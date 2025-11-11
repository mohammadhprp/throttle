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

// CheckLimitResponse contains the response for a check limit request
type CheckLimitResponse struct {
	Allowed    bool  `json:"allowed"`
	Remaining  int64 `json:"remaining"`
	ResetAt    int64 `json:"reset_at"`
	RetryAfter int   `json:"retry_after"`
}

// GetStatusResponse contains the status information for a rate limit
type GetStatusResponse struct {
	Config    *RateLimitConfig `json:"config"`
	CreatedAt int64            `json:"created_at"`
	UpdatedAt int64            `json:"updated_at"`
	NextReset int64            `json:"next_reset"`
}

// RateLimitMetadata contains metadata about a rate limit configuration
type RateLimitMetadata struct {
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
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
		return errors.New(ErrAlgorithmRequired)
	}

	validAlgorithms := map[string]bool{
		AlgorithmTokenBucket:   true,
		AlgorithmSlidingWindow: true,
		AlgorithmFixedWindow:   true,
		AlgorithmLeakyBucket:   true,
	}

	if !validAlgorithms[cfg.Algorithm] {
		return fmt.Errorf(ErrInvalidAlgorithm, cfg.Algorithm)
	}

	if cfg.Limit <= 0 {
		return errors.New(ErrLimitMustBePositive)
	}

	if cfg.WindowSeconds <= 0 {
		return errors.New(ErrWindowSecondsMissing)
	}

	if cfg.Algorithm == AlgorithmTokenBucket {
		if cfg.RefillRate <= 0 {
			return errors.New(ErrRefillRateMissing)
		}
		if cfg.RefillInterval <= 0 {
			return errors.New(ErrRefillIntervalMissing)
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
		return nil, ErrNotFound
	}

	var config RateLimitConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	return &config, nil
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
	case AlgorithmTokenBucket:
		refillInterval := time.Duration(cfg.RefillInterval) * time.Second
		lim = limiter.NewTokenBucket(s.store, float64(cfg.RefillRate), refillInterval, float64(cfg.Limit), s.Logger)

	case AlgorithmSlidingWindow:
		lim = limiter.NewSlidingWindow(s.store, cfg.Limit, windowDuration, s.Logger)

	case AlgorithmFixedWindow:
		lim = limiter.NewFixedWindow(s.store, cfg.Limit, windowDuration, s.Logger)

	case AlgorithmLeakyBucket:
		lim = limiter.NewLeakyBucket(s.store, cfg.Limit, leakRate, windowDuration, s.Logger)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", cfg.Algorithm)
	}

	s.limiters[limiterKey] = lim
	return lim, nil
}

// nextWindowStart calculates the Unix timestamp of the start of the next window
func (s *RateLimitService) nextWindowStart(cfg *RateLimitConfig) int64 {
	now := time.Now()
	windowsSinceEpoch := now.Unix() / int64(cfg.WindowSeconds)
	return (windowsSinceEpoch + 1) * int64(cfg.WindowSeconds)
}

// CalculateRemaining calculates the remaining requests/tokens
// NOTE: This returns a simplified estimate (limit if allowed, 0 if denied).
// For accurate remaining capacity, the RateLimiter interface should expose a Remaining() method.
func (s *RateLimitService) CalculateRemaining(ctx context.Context, key string, cfg *RateLimitConfig, allowed bool) int64 {
	// Simplified calculation based on whether request was allowed
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
	return s.nextWindowStart(cfg)
}

// CalculateRetryAfter calculates seconds to wait before retry
func (s *RateLimitService) CalculateRetryAfter(cfg *RateLimitConfig) int {
	// Calculate seconds until next window
	now := time.Now()
	nextWindowTS := s.nextWindowStart(cfg)
	nextWindowTime := time.Unix(nextWindowTS, 0)
	retryAfter := int(nextWindowTime.Sub(now).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return retryAfter
}

// ConfigKey generates a key for storing configuration
func (s *RateLimitService) ConfigKey(key string) string {
	return ConfigKeyPrefix + key
}

// MetadataKey generates a key for storing metadata
func (s *RateLimitService) MetadataKey(key string) string {
	return MetadataKeyPrefix + key
}

// LimiterKey generates a key for storing limiter instance
func (s *RateLimitService) LimiterKey(key, algorithm string) string {
	return fmt.Sprintf(LimiterKeyFormat, key, algorithm)
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
	metadata := RateLimitMetadata{
		CreatedAt: now.Unix(),
		UpdatedAt: now.Unix(),
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
func (s *RateLimitService) CheckLimit(ctx context.Context, key string) (*CheckLimitResponse, error) {
	// Get configuration
	config, err := s.GetConfig(ctx, key)
	if err != nil {
		return nil, err
	}

	// Get or create limiter
	limiterKey := s.LimiterKey(key, config.Algorithm)
	rateLimiter, err := s.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		return nil, err
	}

	// Check if request is allowed
	allowed, err := rateLimiter.Allow(ctx, key)
	if err != nil {
		// Fail open on error
		allowed = true
	}

	// Calculate metrics for response
	remaining := s.CalculateRemaining(ctx, key, config, allowed)
	resetAt := s.CalculateResetAt(config)
	retryAfter := s.CalculateRetryAfter(config)

	return &CheckLimitResponse{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetAt:    resetAt,
		RetryAfter: retryAfter,
	}, nil
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

	if err := s.store.Delete(ctx, configKey); err != nil {
		s.Logger.Warn("failed to delete rate limit config", zap.String("key", key), zap.Error(err))
	}

	if err := s.store.Delete(ctx, metadataKey); err != nil {
		s.Logger.Warn("failed to delete rate limit metadata", zap.String("key", key), zap.Error(err))
	}

	return nil
}

// GetStatus retrieves the current status of a rate limit
func (s *RateLimitService) GetStatus(ctx context.Context, key string) (*GetStatusResponse, error) {
	// Get configuration
	config, err := s.GetConfig(ctx, key)
	if err != nil {
		return nil, err
	}

	// Get metadata
	metadata, err := s.parseMetadata(ctx, key)
	if err != nil {
		// Use current time as defaults if metadata is not found
		now := time.Now().Unix()
		metadata = &RateLimitMetadata{
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	nextReset := s.CalculateResetAt(config)

	return &GetStatusResponse{
		Config:    config,
		CreatedAt: metadata.CreatedAt,
		UpdatedAt: metadata.UpdatedAt,
		NextReset: nextReset,
	}, nil
}

// parseMetadata parses metadata from storage
func (s *RateLimitService) parseMetadata(ctx context.Context, key string) (*RateLimitMetadata, error) {
	metadataKey := s.MetadataKey(key)
	metadataStr, err := s.store.Get(ctx, metadataKey)
	if err != nil {
		return nil, err
	}

	if metadataStr == "" {
		return nil, ErrNotFound
	}

	var metadata RateLimitMetadata
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}
