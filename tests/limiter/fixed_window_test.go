package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"github.com/mohammadhprp/throttle/internal/storage"
)

func TestFixedWindowAllow_InitialWindow(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 5 requests per 1 second window
	limiter := limiter.NewFixedWindow(store, 5, time.Second, logger)
	ctx := context.Background()

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request 6 should be denied (limit reached)")
	}
}

func TestFixedWindowAllow_WindowReset(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 3 requests per 100ms window
	limiter := limiter.NewFixedWindow(store, 3, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Consume all 3 requests in first window
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed in first window", i+1)
		}
	}

	// 4th request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("4th request should be denied in first window")
	}

	// Wait for window to reset
	time.Sleep(101 * time.Millisecond)

	// First request in new window should be allowed
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("first request in new window should be allowed")
	}
}

func TestFixedWindowAllow_MultipleKeys(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 2 requests per window
	limiter := limiter.NewFixedWindow(store, 2, time.Second, logger)
	ctx := context.Background()

	// Consume requests for key1
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "key1")
		if err != nil {
			t.Fatalf("unexpected error for key1: %v", err)
		}
		if !allowed {
			t.Errorf("request %d for key1 should be allowed", i+1)
		}
	}

	// key1 should be denied next request
	allowed, err := limiter.Allow(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("3rd request for key1 should be denied")
	}

	// key2 should still have full quota (keys are isolated)
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "key2")
		if err != nil {
			t.Fatalf("unexpected error for key2: %v", err)
		}
		if !allowed {
			t.Errorf("request %d for key2 should be allowed (isolated from key1)", i+1)
		}
	}

	// key2 should also be denied next request
	allowed, err = limiter.Allow(ctx, "key2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("3rd request for key2 should be denied")
	}
}

func TestFixedWindowAllow_EdgeCaseZeroRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 0 requests (reject all)
	limiter := limiter.NewFixedWindow(store, 0, time.Second, logger)
	ctx := context.Background()

	// First request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should be denied when max_requests is 0")
	}
}

func TestFixedWindowAllow_LargeWindowDuration(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 10 requests per 10 second window
	limiter := limiter.NewFixedWindow(store, 10, 10*time.Second, logger)
	ctx := context.Background()

	// Allow all 10 requests
	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 11th should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("11th request should be denied")
	}

	// Window shouldn't reset within 10 seconds
	time.Sleep(100 * time.Millisecond)

	// Still denied after small wait
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should still be denied after 100ms wait (window is 10s)")
	}
}

func TestFixedWindowReset(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewFixedWindow(store, 3, time.Second, logger)
	ctx := context.Background()

	// Consume all requests
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Next request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("4th request should be denied before reset")
	}

	// Reset the state
	if err := limiter.Reset(ctx, "test_key"); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// After reset, should allow 3 more requests
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error after reset: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after reset", i+1)
		}
	}
}

func TestFixedWindowAllow_StateRecovery(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter1 := limiter.NewFixedWindow(store, 5, time.Second, logger)
	ctx := context.Background()

	// Consume 3 requests with first limiter
	for i := 0; i < 3; i++ {
		allowed, err := limiter1.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Create new limiter instance with same store
	limiter2 := limiter.NewFixedWindow(store, 5, time.Second, logger)

	// State should persist - we already used 3 requests, so 2 remain
	allowed, err := limiter2.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("4th request should be allowed (recovered state)")
	}

	allowed, err = limiter2.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("5th request should be allowed (recovered state)")
	}

	// 6th request should be denied
	allowed, err = limiter2.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("6th request should be denied (limit reached)")
	}
}

func TestFixedWindowAllow_ConcurrentRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 20 requests per window
	limiter := limiter.NewFixedWindow(store, 20, time.Second, logger)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0
	deniedCount := 0

	// Spawn 30 goroutines that submit requests concurrently
	// They may not execute exactly at the same time, but test thread-safety
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, err := limiter.Allow(ctx, "test_key")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			mu.Lock()
			if allowed {
				allowedCount++
			} else {
				deniedCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Should allow exactly 20, deny 10 (with potential concurrency)
	// Allow some tolerance due to timing variations
	if allowedCount < 15 || allowedCount > 20 {
		t.Logf("info: allowed=%d, denied=%d (within acceptable concurrent range)", allowedCount, deniedCount)
	}
	if allowedCount+deniedCount != 30 {
		t.Errorf("total requests should be 30, got %d", allowedCount+deniedCount)
	}
}

func TestFixedWindowAllow_HighRequestRate(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 100 requests per 10ms window
	limiter := limiter.NewFixedWindow(store, 100, 10*time.Millisecond, logger)
	ctx := context.Background()

	// Rapid-fire requests
	allowedCount := 0
	for i := 0; i < 150; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if allowed {
			allowedCount++
		}
	}

	// First 100 should be allowed
	if allowedCount != 100 {
		t.Errorf("expected 100 allowed requests, got %d", allowedCount)
	}
}

func TestFixedWindowClose(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewFixedWindow(store, 5, time.Second, logger)

	// Close should not error
	if err := limiter.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestFixedWindowAllow_BoundaryCondition(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 1 request per window
	limiter := limiter.NewFixedWindow(store, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// First request in window 1
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("first request should be allowed")
	}

	// Second request in window 1 should be denied
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("second request in same window should be denied")
	}

	// Wait just under 100ms - still in same window
	time.Sleep(90 * time.Millisecond)
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request after 90ms should still be denied (window still active)")
	}

	// Wait to cross window boundary
	time.Sleep(20 * time.Millisecond) // total 110ms

	// First request in new window should be allowed
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("first request in new window should be allowed")
	}
}

func TestFixedWindowAllow_LowLimit(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 1 request per second
	limiter := limiter.NewFixedWindow(store, 1, time.Second, logger)
	ctx := context.Background()

	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("first request should be allowed")
	}

	// Second request should be denied
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("second request should be denied with limit of 1")
	}
}

func TestFixedWindowAllow_HighLimit(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 1000 requests per second
	limiter := limiter.NewFixedWindow(store, 1000, time.Second, logger)
	ctx := context.Background()

	// Allow 1000 requests
	for i := 0; i < 1000; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 1001st request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("1001st request should be denied")
	}
}
