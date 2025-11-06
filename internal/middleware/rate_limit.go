package middleware

import (
	"net/http"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"go.uber.org/zap"
)

// RateLimitMiddleware returns an HTTP middleware that applies rate limiting to requests.
// It uses the provided RateLimiter to determine if a request should be allowed.
//
// The identifier (client/user key) is extracted using the provided keyExtractor function.
// If the request is rate limited, a 429 Too Many Requests response is returned.
//
// Parameters:
// - limiter: The rate limiting algorithm (e.g., TokenBucket)
// - keyExtractor: Function to extract the identifier from the request (e.g., IP address, user ID)
// - logger: Logger instance for debugging
//
// Example: Rate limit by IP address
//   middleware := RateLimitMiddleware(tokenBucket, func(r *http.Request) string {
//       return r.RemoteAddr
//   }, logger)
func RateLimitMiddleware(limiter limiter.RateLimiter, keyExtractor func(*http.Request) string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the identifier for this request
			key := keyExtractor(r)

			// Check if the request is allowed under the rate limit
			allowed, err := limiter.Allow(r.Context(), key)
			if err != nil {
				logger.Error("rate limiter check failed", zap.String("key", key), zap.Error(err))
				// Fail open: allow the request if rate limiter fails
				next.ServeHTTP(w, r)
				return
			}

			// If not allowed, return 429 Too Many Requests
			if !allowed {
				logger.Debug("request rate limited", zap.String("key", key))
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("rate limit exceeded"))
				return
			}

			// Request is allowed, continue to the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// IPKeyExtractor extracts the client IP address from the request.
// It checks for X-Forwarded-For header first (for proxied requests),
// then falls back to RemoteAddr.
func IPKeyExtractor(r *http.Request) string {
	// Check for X-Forwarded-For header (set by proxies)
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return forwardedFor
	}

	// Fall back to remote address
	return r.RemoteAddr
}

// UserIDKeyExtractor returns a key extractor that uses a custom header for user identification.
// This is useful for authenticated APIs where you want to rate limit per user instead of IP.
func UserIDKeyExtractor(headerName string) func(*http.Request) string {
	return func(r *http.Request) string {
		if userID := r.Header.Get(headerName); userID != "" {
			return userID
		}
		// Fall back to IP if no user ID header
		return IPKeyExtractor(r)
	}
}

// PathKeyExtractor returns a key extractor that combines the request path with the IP.
// This allows different rate limits for different endpoints.
func PathKeyExtractor(r *http.Request) string {
	return r.RemoteAddr + ":" + r.URL.Path
}
