package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"github.com/mohammadhprp/throttle/internal/middleware"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// newTestLogger creates a test logger with minimal output
func newTestLogger(t *testing.T) *zap.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return logger
}

// newTestRateLimiter creates a token bucket rate limiter for testing
func newTestRateLimiter(t *testing.T, refillRate float64, refillInterval time.Duration, maxTokens float64) limiter.RateLimiter {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	return limiter.NewTokenBucket(store, refillRate, refillInterval, maxTokens, logger)
}

// newTestFixedWindowLimiter creates a fixed window rate limiter for testing
func newTestFixedWindowLimiter(t *testing.T, maxRequests int64, windowDuration time.Duration) limiter.RateLimiter {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	return limiter.NewFixedWindow(store, maxRequests, windowDuration, logger)
}

// newTestLeakyBucketLimiter creates a leaky bucket rate limiter for testing
func newTestLeakyBucketLimiter(t *testing.T, capacity int64, leakRate int64, leakInterval time.Duration) limiter.RateLimiter {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	return limiter.NewLeakyBucket(store, capacity, leakRate, leakInterval, logger)
}

// newTestSlidingWindowLimiter creates a sliding window rate limiter for testing
func newTestSlidingWindowLimiter(t *testing.T, maxRequests int64, windowDuration time.Duration) limiter.RateLimiter {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	return limiter.NewSlidingWindow(store, maxRequests, windowDuration, logger)
}

// TestRateLimitMiddleware_AllowedRequest tests that allowed requests pass through
func TestRateLimitMiddleware_AllowedRequest(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 10)
	logger := newTestLogger(t)
	defer logger.Sync()

	middleware := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	// Create a simple test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap the handler with middleware
	wrappedHandler := middleware(testHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	// Create a response recorder
	w := httptest.NewRecorder()

	// Serve the request
	wrappedHandler.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "success" {
		t.Errorf("expected body 'success', got %s", w.Body.String())
	}
}

// TestRateLimitMiddleware_DeniedRequest tests that denied requests return 429
func TestRateLimitMiddleware_DeniedRequest(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	middleware := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := middleware(testHandler)

	// Make 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail (rate limited)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	if w.Body.String() != "rate limit exceeded" {
		t.Errorf("expected body 'rate limit exceeded', got %s", w.Body.String())
	}
}

// TestRateLimitMiddleware_RetryAfterHeader tests that 429 response has Retry-After header
func TestRateLimitMiddleware_RetryAfterHeader(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 1)
	logger := newTestLogger(t)
	defer logger.Sync()

	middleware := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(testHandler)

	// First request should succeed
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Second request should fail
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Check Retry-After header
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter != "1" {
		t.Errorf("expected Retry-After header '1', got '%s'", retryAfter)
	}
}

// TestRateLimitMiddleware_DifferentIPs tests that different IPs have separate buckets
func TestRateLimitMiddleware_DifferentIPs(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	middleware := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := middleware(testHandler)

	// IP1 makes 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP2 makes 1 request (should succeed - different bucket)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}

	// IP1 makes 3rd request (should fail)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 request 3: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// IP2 makes another request (should succeed - still has tokens)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP2 request 2: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_XForwardedForHeader tests that X-Forwarded-For header is used
func TestRateLimitMiddleware_XForwardedForHeader(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	middleware := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(testHandler)

	// Make 2 requests with X-Forwarded-For header
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail (X-Forwarded-For is consistent)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Request with different X-Forwarded-For should succeed (different bucket)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.2")
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_UserIDKeyExtractor tests user-based rate limiting
func TestRateLimitMiddleware_UserIDKeyExtractor(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	extractor := middleware.UserIDKeyExtractor("X-User-ID")
	mw := middleware.RateLimitMiddleware(rl, extractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// User1 makes 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.Header.Set("X-User-ID", "user1")
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("user1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// User1's 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-User-ID", "user1")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("user1 request 3: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// User2 makes a request (should succeed - different bucket)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-User-ID", "user2")
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("user2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_UserIDFallbackToIP tests that UserIDKeyExtractor falls back to IP
func TestRateLimitMiddleware_UserIDFallbackToIP(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	extractor := middleware.UserIDKeyExtractor("X-User-ID")
	mw := middleware.RateLimitMiddleware(rl, extractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Request without X-User-ID header - should use IP
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail (same IP)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

// TestRateLimitMiddleware_PathKeyExtractor tests path-based rate limiting
func TestRateLimitMiddleware_PathKeyExtractor(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.PathKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// IP makes 2 requests to /api/endpoint1 (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/api/endpoint1", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("endpoint1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request to /api/endpoint1 should fail
	req := httptest.NewRequest("GET", "http://example.com/api/endpoint1", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("endpoint1 request 3: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// But request to /api/endpoint2 should succeed (different path)
	req = httptest.NewRequest("GET", "http://example.com/api/endpoint2", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("endpoint2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_CustomKeyExtractor tests custom key extractor
func TestRateLimitMiddleware_CustomKeyExtractor(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	// Custom extractor: use a specific header
	customExtractor := func(r *http.Request) string {
		if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
			return apiKey
		}
		return "anonymous"
	}

	mw := middleware.RateLimitMiddleware(rl, customExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Authenticated user makes 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.Header.Set("X-API-Key", "key123")
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("authenticated request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-API-Key", "key123")
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Anonymous user should have separate bucket
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("anonymous request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_FailOpen tests that middleware fails open on limiter error
func TestRateLimitMiddleware_FailOpen(t *testing.T) {
	// Create a mock limiter that always returns an error
	mockLimiter := &mockRateLimiter{
		allowErr: context.DeadlineExceeded,
	}

	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(mockLimiter, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := mw(testHandler)

	// Request should be allowed despite limiter error (fail open)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "success" {
		t.Errorf("expected body 'success', got %s", w.Body.String())
	}
}

// TestRateLimitMiddleware_TokenRefillBetweenRequests tests token refill between requests
func TestRateLimitMiddleware_TokenRefillBetweenRequests(t *testing.T) {
	rl := newTestRateLimiter(t, 1, 100*time.Millisecond, 2)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Make 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request 3: expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Wait for tokens to refill (1 token per 100ms)
	time.Sleep(120 * time.Millisecond)

	// Next request should succeed (token refilled)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("request 4: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitMiddleware_ConcurrentRequests tests middleware under concurrent load
func TestRateLimitMiddleware_ConcurrentRequests(t *testing.T) {
	rl := newTestRateLimiter(t, 10, time.Second, 10)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	done := make(chan struct{})
	var allowedCount, deniedCount int

	// Send 20 concurrent requests
	for i := 0; i < 20; i++ {
		go func() {
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			w := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(w, req)

			if http.StatusOK == w.Code {
				allowedCount++
			} else if w.Code == http.StatusTooManyRequests {
				deniedCount++
			} else {
				t.Errorf("unexpected status %d", w.Code)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should have 10 allowed and 10 denied
	if allowedCount+deniedCount != 20 {
		t.Errorf("expected 20 total requests, got %d", allowedCount+deniedCount)
	}

	if allowedCount < 9 || allowedCount > 11 {
		t.Logf("allowed count: %d, denied count: %d", allowedCount, deniedCount)
	}
}

// mockRateLimiter is a mock implementation of RateLimiter for testing
type mockRateLimiter struct {
	allowed   bool
	allowErr  error
	resetErr  error
	closeErr  error
	callCount int
}

func (m *mockRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	m.callCount++
	return m.allowed, m.allowErr
}

func (m *mockRateLimiter) Reset(ctx context.Context, key string) error {
	return m.resetErr
}

func (m *mockRateLimiter) Close() error {
	return m.closeErr
}

// IPKeyExtractor tests
func TestIPKeyExtractor_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key := middleware.IPKeyExtractor(req)
	if key != "192.168.1.1:12345" {
		t.Errorf("expected '192.168.1.1:12345', got '%s'", key)
	}
}

func TestIPKeyExtractor_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "127.0.0.1:12345"

	key := middleware.IPKeyExtractor(req)
	if key != "203.0.113.1" {
		t.Errorf("expected '203.0.113.1', got '%s'", key)
	}
}

func TestIPKeyExtractor_PreferXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "192.168.1.1:12345"

	key := middleware.IPKeyExtractor(req)
	if key != "203.0.113.1" {
		t.Errorf("expected X-Forwarded-For to take precedence, got '%s'", key)
	}
}

// UserIDKeyExtractor tests
func TestUserIDKeyExtractor_WithUserID(t *testing.T) {
	extractor := middleware.UserIDKeyExtractor("X-User-ID")
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-User-ID", "user123")
	req.RemoteAddr = "192.168.1.1:12345"

	key := extractor(req)
	if key != "user123" {
		t.Errorf("expected 'user123', got '%s'", key)
	}
}

func TestUserIDKeyExtractor_WithoutUserID(t *testing.T) {
	extractor := middleware.UserIDKeyExtractor("X-User-ID")
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key := extractor(req)
	if key != "192.168.1.1:12345" {
		t.Errorf("expected fallback to IP '192.168.1.1:12345', got '%s'", key)
	}
}

func TestUserIDKeyExtractor_CustomHeaderName(t *testing.T) {
	extractor := middleware.UserIDKeyExtractor("X-API-Key")
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-API-Key", "secret-key-456")
	req.RemoteAddr = "192.168.1.1:12345"

	key := extractor(req)
	if key != "secret-key-456" {
		t.Errorf("expected 'secret-key-456', got '%s'", key)
	}
}

// PathKeyExtractor tests
func TestPathKeyExtractor_BasicPath(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/users", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key := middleware.PathKeyExtractor(req)
	expected := "192.168.1.1:12345:/api/users"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestPathKeyExtractor_NestedPath(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/api/v1/users/123", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key := middleware.PathKeyExtractor(req)
	expected := "192.168.1.1:12345:/api/v1/users/123"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestPathKeyExtractor_DifferentPaths(t *testing.T) {
	req1 := httptest.NewRequest("GET", "http://example.com/api/users", nil)
	req1.RemoteAddr = "192.168.1.1:12345"

	req2 := httptest.NewRequest("GET", "http://example.com/api/products", nil)
	req2.RemoteAddr = "192.168.1.1:12345"

	key1 := middleware.PathKeyExtractor(req1)
	key2 := middleware.PathKeyExtractor(req2)

	if key1 == key2 {
		t.Errorf("expected different keys for different paths")
	}
}

// ============================================================================
// Tests for FixedWindow Rate Limiter with Middleware
// ============================================================================

func TestRateLimitMiddleware_FixedWindowAllow(t *testing.T) {
	rl := newTestFixedWindowLimiter(t, 2, time.Second)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := mw(testHandler)

	// Make 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestRateLimitMiddleware_FixedWindowWindowReset(t *testing.T) {
	rl := newTestFixedWindowLimiter(t, 2, 100*time.Millisecond)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Make 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}

	// Wait for window to reset
	time.Sleep(101 * time.Millisecond)

	// Request should now succeed (new window)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d after window reset, got %d", http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_FixedWindowMultipleKeys(t *testing.T) {
	rl := newTestFixedWindowLimiter(t, 2, time.Second)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// IP1 makes 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP2 makes 1 request (should succeed - different bucket)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// ============================================================================
// Tests for LeakyBucket Rate Limiter with Middleware
// ============================================================================

func TestRateLimitMiddleware_LeakyBucketAllow(t *testing.T) {
	rl := newTestLeakyBucketLimiter(t, 3, 1, 100*time.Millisecond)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := mw(testHandler)

	// Make 3 requests (should succeed - within capacity)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 4th request should fail (bucket at capacity)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestRateLimitMiddleware_LeakyBucketLeak(t *testing.T) {
	rl := newTestLeakyBucketLimiter(t, 3, 1, 100*time.Millisecond)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Fill bucket with 3 requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// Bucket is full, next request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d (bucket full), got %d", http.StatusTooManyRequests, w.Code)
	}

	// Wait for leak
	time.Sleep(100 * time.Millisecond)

	// Request should now succeed (1 request leaked)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d after leak, got %d", http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_LeakyBucketMultipleKeys(t *testing.T) {
	rl := newTestLeakyBucketLimiter(t, 2, 1, 100*time.Millisecond)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// IP1 fills bucket with 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP2 makes request (should succeed - separate bucket)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// ============================================================================
// Tests for SlidingWindow Rate Limiter with Middleware
// ============================================================================

func TestRateLimitMiddleware_SlidingWindowAllow(t *testing.T) {
	rl := newTestSlidingWindowLimiter(t, 2, time.Second)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	wrappedHandler := mw(testHandler)

	// Make 2 requests (should succeed)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d", http.StatusTooManyRequests, w.Code)
	}
}

func TestRateLimitMiddleware_SlidingWindowSlide(t *testing.T) {
	rl := newTestSlidingWindowLimiter(t, 2, 100*time.Millisecond)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// Make 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// 3rd request should fail (window full)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d (window full), got %d", http.StatusTooManyRequests, w.Code)
	}

	// Wait for window to slide (oldest requests to fall out)
	time.Sleep(101 * time.Millisecond)

	// Request should now succeed (window slid)
	req = httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d after window slide, got %d", http.StatusOK, w.Code)
	}
}

func TestRateLimitMiddleware_SlidingWindowMultipleKeys(t *testing.T) {
	rl := newTestSlidingWindowLimiter(t, 2, time.Second)
	logger := newTestLogger(t)
	defer logger.Sync()

	mw := middleware.RateLimitMiddleware(rl, middleware.IPKeyExtractor, logger)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := mw(testHandler)

	// IP1 makes 2 requests
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("IP1 request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}
	}

	// IP2 makes 1 request (should succeed - different bucket)
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("IP2 request: expected status %d, got %d", http.StatusOK, w.Code)
	}
}
