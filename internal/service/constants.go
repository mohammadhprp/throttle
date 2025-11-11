package service

import "errors"

// Algorithm types
const (
	AlgorithmTokenBucket   = "token_bucket"
	AlgorithmSlidingWindow = "sliding_window"
	AlgorithmFixedWindow   = "fixed_window"
	AlgorithmLeakyBucket   = "leaky_bucket"
)

// Storage key prefixes
const (
	ConfigKeyPrefix    = "ratelimit:config:"
	MetadataKeyPrefix  = "ratelimit:metadata:"
	LimiterKeyFormat   = "%s:%s"
)

// Health status constants
const (
	HealthStatusHealthy   = "healthy"
	HealthStatusUnhealthy = "unhealthy"
)

// Validation error messages
const (
	ErrAlgorithmRequired     = "algorithm is required"
	ErrInvalidAlgorithm      = "invalid algorithm: %s"
	ErrLimitMustBePositive   = "limit must be greater than 0"
	ErrWindowSecondsMissing  = "window_seconds must be greater than 0"
	ErrRefillRateMissing     = "refill_rate must be greater than 0 for token bucket"
	ErrRefillIntervalMissing = "refill_interval must be greater than 0 for token bucket"
	ErrConfigurationNotFound = "configuration not found"
)

// Custom error types
var (
	ErrNotFound = errors.New(ErrConfigurationNotFound)
)
