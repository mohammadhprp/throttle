package limiter

import "context"

// RateLimiter defines the interface for rate limiting algorithms.
// Different algorithms can be plugged in (Token Bucket, Leaky Bucket, Sliding Window, Fixed Window)
// Error Handling Behavior (Fail-Open)
type RateLimiter interface {
	// Allow checks if a request should be allowed based on the rate limiting algorithm.
	// Returns true if the request is allowed, false otherwise.
	// The key parameter typically identifies the client/user/IP being rate limited.
	//
	// On error (e.g., storage failure), returns (true, error) to implement fail-open behavior.
	Allow(ctx context.Context, key string) (bool, error)

	// Reset clears the state for a specific key.
	Reset(ctx context.Context, key string) error

	// Close performs cleanup when the rate limiter is no longer needed.
	Close() error
}
