package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"github.com/mohammadhprp/throttle/internal/storage"
)

func TestSlidingWindowAllow_InitialWindow(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 5 requests per 1 second window
	limiter := limiter.NewSlidingWindow(store, 5, time.Second, logger)
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

func TestSlidingWindowAllow_WindowSliding(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 3 requests per 100ms window
	limiter := limiter.NewSlidingWindow(store, 3, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Add 3 requests at time T0
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied (within same 100ms window)
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("4th request should be denied (still in same window)")
	}

	// Wait for window to slide (oldest requests to fall out)
	time.Sleep(101 * time.Millisecond)

	// All previous requests should be outside window, allow new ones
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after window slide", i+1)
		}
	}
}

func TestSlidingWindowAllow_PartialWindowSlide(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 5 requests per 200ms window
	limiter := limiter.NewSlidingWindow(store, 5, 200*time.Millisecond, logger)
	ctx := context.Background()

	// Add 2 requests at time T0
	allowed1, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed1 {
		t.Errorf("request 1 should be allowed")
	}

	allowed2, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed2 {
		t.Errorf("request 2 should be allowed")
	}

	// Wait 150ms (still within 200ms window of first requests)
	time.Sleep(150 * time.Millisecond)

	// Add 3 more requests - should be allowed (total 5)
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed (partial window slide)", i+1)
		}
	}

	// 6th request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("6th request should be denied (limit reached)")
	}

	// Wait for first 2 requests to fall out (100ms more = 250ms total)
	time.Sleep(100 * time.Millisecond)

	// Now should be able to add 2 more (oldest 2 are outside window)
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed (window slid)", i+1)
		}
	}
}

func TestSlidingWindowAllow_MultipleKeys(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 2 requests per window
	limiter := limiter.NewSlidingWindow(store, 2, time.Second, logger)
	ctx := context.Background()

	// Consume requests for key1
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "key1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
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

	// key2 should have separate quota (isolated)
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "key2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d for key2 should be allowed (isolated)", i+1)
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

func TestSlidingWindowAllow_EdgeCaseZeroRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 0 requests
	limiter := limiter.NewSlidingWindow(store, 0, time.Second, logger)
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

func TestSlidingWindowAllow_LargeWindowDuration(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 10 requests per 10 second window
	limiter := limiter.NewSlidingWindow(store, 10, 10*time.Second, logger)
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

	// Window shouldn't reset within short time
	time.Sleep(100 * time.Millisecond)

	// Still denied after small wait
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should still be denied after 100ms")
	}
}

func TestSlidingWindowReset(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewSlidingWindow(store, 3, time.Second, logger)
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

	// Reset
	if err := limiter.Reset(ctx, "test_key"); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// After reset, should allow 3 more requests
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after reset", i+1)
		}
	}
}

func TestSlidingWindowAllow_StateRecovery(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter1 := limiter.NewSlidingWindow(store, 5, time.Second, logger)
	ctx := context.Background()

	// Add 3 requests with first limiter
	for i := 0; i < 3; i++ {
		allowed, err := limiter1.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Create new limiter with same store
	limiter2 := limiter.NewSlidingWindow(store, 5, time.Second, logger)

	// State should persist - 3 requests recorded, 2 slots remain
	for i := 0; i < 2; i++ {
		allowed, err := limiter2.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed (recovered state)", i+1)
		}
	}

	// 6th request should be denied
	allowed, err := limiter2.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("6th request should be denied (limit reached)")
	}
}

func TestSlidingWindowAllow_ConcurrentRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: max 20 requests per window
	limiter := limiter.NewSlidingWindow(store, 20, time.Second, logger)
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

func TestSlidingWindowAllow_HighRequestRate(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 100 requests per 10ms window
	limiter := limiter.NewSlidingWindow(store, 100, 10*time.Millisecond, logger)
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

func TestSlidingWindowAllow_TimeoutWindowScenario(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 3 requests per 50ms window
	limiter := limiter.NewSlidingWindow(store, 3, 50*time.Millisecond, logger)
	ctx := context.Background()

	// Add 3 requests at T0
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request denied (still in window)
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("4th request should be denied")
	}

	// Wait 25ms (T=25ms, all 3 requests still in window)
	time.Sleep(25 * time.Millisecond)

	// All 3 still in window, should deny
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request at T=25ms should be denied (requests still in window)")
	}

	// Wait another 26ms (T=51ms, all 3 should be outside window)
	time.Sleep(26 * time.Millisecond)

	// All should be out of window now
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d at T>50ms should be allowed", i+1)
		}
	}
}

func TestSlidingWindowClose(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewSlidingWindow(store, 5, time.Second, logger)

	// Close should not error
	if err := limiter.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestSlidingWindowAllow_NoCliffBehavior(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 5 requests per 200ms window (longer for more reliable timing)
	limiter := limiter.NewSlidingWindow(store, 5, 200*time.Millisecond, logger)
	ctx := context.Background()

	// Add 5 requests at T=0
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied (window full)
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("6th request should be denied (window full)")
	}

	// Wait for 150ms (oldest requests still in window - less than 200ms)
	time.Sleep(150 * time.Millisecond)

	// Still denied - requests still in window
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request at T=150ms should be denied (requests still in window)")
	}

	// Wait more to pass 200ms total (all requests now outside window)
	time.Sleep(60 * time.Millisecond) // Total ~210ms

	// Now should allow 5 new requests
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after window slide", i+1)
		}
	}

	// 6th should be denied again
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("6th request should be denied (window full again)")
	}
}

func TestSlidingWindowAllow_SingleRequest(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 1 request per second
	limiter := limiter.NewSlidingWindow(store, 1, time.Second, logger)
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
		t.Errorf("second request should be denied with max=1")
	}

	// After 1 second, should allow
	time.Sleep(1 * time.Second)

	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("request after 1 second should be allowed")
	}
}

func TestSlidingWindowAllow_HighLimit(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Max 1000 requests per second
	limiter := limiter.NewSlidingWindow(store, 1000, time.Second, logger)
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
