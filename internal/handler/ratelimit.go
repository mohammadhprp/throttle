package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
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

// CheckRequest represents a rate limit check request
type CheckRequest struct {
	Key       string `json:"key"`
	Algorithm string `json:"algorithm"`
}

// CheckResponse represents a rate limit check response
type CheckResponse struct {
	Allowed    bool  `json:"allowed"`
	Remaining  int64 `json:"remaining"`
	ResetAt    int64 `json:"reset_at"`    // Unix timestamp
	RetryAfter int   `json:"retry_after"` // seconds
}

// StatusResponse represents the status of a rate limit
type StatusResponse struct {
	Key       string      `json:"key"`
	Algorithm string      `json:"algorithm"`
	Config    interface{} `json:"config"`
	CreatedAt int64       `json:"created_at"` // Unix timestamp
	UpdatedAt int64       `json:"updated_at"` // Unix timestamp
	NextReset int64       `json:"next_reset"` // Unix timestamp
}

// RateLimitHandler handles rate limit operations
type RateLimitHandler struct {
	store    storage.Store
	logger   *zap.Logger
	limiters map[string]limiter.RateLimiter
}

// NewRateLimitHandler creates a new rate limit handler
func NewRateLimitHandler(store storage.Store, logger *zap.Logger) *RateLimitHandler {
	return &RateLimitHandler{
		store:    store,
		logger:   logger,
		limiters: make(map[string]limiter.RateLimiter),
	}
}

// Set handles POST /ratelimit/set - configure rate limits
func (h *RateLimitHandler) Set() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key    string          `json:"key"`
			Config RateLimitConfig `json:"config"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := h.validateConfig(req.Config); err != nil {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Store configuration in Redis
		configKey := h.configKey(req.Key)
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			h.logger.Error("failed to marshal config", zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to save configuration")
			return
		}

		now := time.Now()
		metadata := map[string]interface{}{
			"config":     req.Config,
			"created_at": now.Unix(),
			"updated_at": now.Unix(),
		}

		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			h.logger.Error("failed to marshal metadata", zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to save configuration")
			return
		}

		// Store both config and metadata
		if err := h.store.Set(r.Context(), configKey, string(configJSON), 0); err != nil {
			h.logger.Error("failed to store config", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to save configuration")
			return
		}

		metadataKey := h.metadataKey(req.Key)
		if err := h.store.Set(r.Context(), metadataKey, string(metadataJSON), 0); err != nil {
			h.logger.Error("failed to store metadata", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to save configuration")
			return
		}

		h.logger.Info("rate limit configured", zap.String("key", req.Key), zap.String("algorithm", req.Config.Algorithm))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "rate limit configured",
			"key":     req.Key,
		})
	}
}

// Check handles POST /ratelimit/check - verify if request is allowed
func (h *RateLimitHandler) Check() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CheckRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		// Get configuration
		config, err := h.getConfig(r.Context(), req.Key)
		if err != nil {
			h.logger.Error("failed to get config", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		// Get or create limiter
		limiterKey := h.limiterKey(req.Key, config.Algorithm)
		rateLimiter, err := h.getOrCreateLimiter(limiterKey, config)
		if err != nil {
			h.logger.Error("failed to create limiter", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		// Check if request is allowed
		allowed, err := rateLimiter.Allow(r.Context(), req.Key)
		if err != nil {
			h.logger.Error("failed to check rate limit", zap.String("key", req.Key), zap.Error(err))
			// Fail open on error
			allowed = true
		}

		// Calculate metrics for response
		remaining := h.calculateRemaining(r.Context(), req.Key, config, allowed)
		resetAt := h.calculateResetAt(config)
		retryAfter := h.calculateRetryAfter(config)

		resp := CheckResponse{
			Allowed:    allowed,
			Remaining:  remaining,
			ResetAt:    resetAt,
			RetryAfter: retryAfter,
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", config.Limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))
		if !allowed {
			w.Header().Set("X-RateLimit-Retry-After", fmt.Sprintf("%d", retryAfter))
		}
		w.Header().Set("Content-Type", "application/json")

		if !allowed {
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(resp)
	}
}

// Status handles GET /ratelimit/status/:key - get status of a rate limit
func (h *RateLimitHandler) Status() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]

		if key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		// Get configuration
		config, err := h.getConfig(r.Context(), key)
		if err != nil {
			h.logger.Error("failed to get config", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		// Get metadata
		metadata, err := h.getMetadata(r.Context(), key)
		if err != nil {
			h.logger.Error("failed to get metadata", zap.String("key", key), zap.Error(err))
			metadata = map[string]interface{}{
				"created_at": time.Now().Unix(),
				"updated_at": time.Now().Unix(),
			}
		}

		createdAt := int64(0)
		updatedAt := int64(0)
		if ca, ok := metadata["created_at"].(float64); ok {
			createdAt = int64(ca)
		}
		if ua, ok := metadata["updated_at"].(float64); ok {
			updatedAt = int64(ua)
		}

		resp := StatusResponse{
			Key:       key,
			Algorithm: config.Algorithm,
			Config:    config,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			NextReset: h.calculateResetAt(config),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}

// Reset handles DELETE /ratelimit/reset/:key - reset a rate limit
func (h *RateLimitHandler) Reset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]

		if key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		// Get configuration to find limiter type
		config, err := h.getConfig(r.Context(), key)
		if err != nil {
			h.logger.Error("failed to get config", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		// Get or create limiter and reset it
		limiterKey := h.limiterKey(key, config.Algorithm)
		rateLimiter, err := h.getOrCreateLimiter(limiterKey, config)
		if err != nil {
			h.logger.Error("failed to create limiter", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		if err := rateLimiter.Reset(r.Context(), key); err != nil {
			h.logger.Error("failed to reset rate limit", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to reset rate limit")
			return
		}

		// Delete configuration and metadata
		configKey := h.configKey(key)
		metadataKey := h.metadataKey(key)

		_ = h.store.Delete(r.Context(), configKey)
		_ = h.store.Delete(r.Context(), metadataKey)

		h.logger.Info("rate limit reset", zap.String("key", key))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "rate limit reset",
			"key":     key,
		})
	}
}

// Helper methods

// validateConfig validates the rate limit configuration
func (h *RateLimitHandler) validateConfig(cfg RateLimitConfig) error {
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

// getConfig retrieves the configuration for a key
func (h *RateLimitHandler) getConfig(ctx context.Context, key string) (*RateLimitConfig, error) {
	configKey := h.configKey(key)
	configStr, err := h.store.Get(ctx, configKey)
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

// getMetadata retrieves the metadata for a key
func (h *RateLimitHandler) getMetadata(ctx context.Context, key string) (map[string]interface{}, error) {
	metadataKey := h.metadataKey(key)
	metadataStr, err := h.store.Get(ctx, metadataKey)
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

// getOrCreateLimiter gets an existing limiter or creates a new one
func (h *RateLimitHandler) getOrCreateLimiter(limiterKey string, cfg *RateLimitConfig) (limiter.RateLimiter, error) {
	if lim, ok := h.limiters[limiterKey]; ok {
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
		lim = limiter.NewTokenBucket(h.store, float64(cfg.RefillRate), refillInterval, float64(cfg.Limit), h.logger)

	case "sliding_window":
		lim = limiter.NewSlidingWindow(h.store, cfg.Limit, windowDuration, h.logger)

	case "fixed_window":
		lim = limiter.NewFixedWindow(h.store, cfg.Limit, windowDuration, h.logger)

	case "leaky_bucket":
		lim = limiter.NewLeakyBucket(h.store, cfg.Limit, leakRate, windowDuration, h.logger)

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", cfg.Algorithm)
	}

	h.limiters[limiterKey] = lim
	return lim, nil
}

// calculateRemaining calculates the remaining requests/tokens
func (h *RateLimitHandler) calculateRemaining(ctx context.Context, key string, cfg *RateLimitConfig, allowed bool) int64 {
	// Simplified calculation based on algorithm
	// This is a best-effort estimate since actual remaining depends on algorithm internals
	remaining := cfg.Limit
	if !allowed {
		remaining = 0
	}
	return remaining
}

// calculateResetAt calculates the reset timestamp
func (h *RateLimitHandler) calculateResetAt(cfg *RateLimitConfig) int64 {
	// Window-based algorithms reset at the start of the next window
	now := time.Now()
	windowsSinceEpoch := now.Unix() / int64(cfg.WindowSeconds)
	nextWindowStart := (windowsSinceEpoch + 1) * int64(cfg.WindowSeconds)
	return nextWindowStart
}

// calculateRetryAfter calculates seconds to wait before retry
func (h *RateLimitHandler) calculateRetryAfter(cfg *RateLimitConfig) int {
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

// configKey generates a key for storing configuration
func (h *RateLimitHandler) configKey(key string) string {
	return fmt.Sprintf("ratelimit:config:%s", key)
}

// metadataKey generates a key for storing metadata
func (h *RateLimitHandler) metadataKey(key string) string {
	return fmt.Sprintf("ratelimit:metadata:%s", key)
}

// limiterKey generates a key for storing limiter instance
func (h *RateLimitHandler) limiterKey(key, algorithm string) string {
	return fmt.Sprintf("%s:%s", key, algorithm)
}

// writeError writes an error response
func (h *RateLimitHandler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
