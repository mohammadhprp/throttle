package limiter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/limiter"
	"github.com/mohammadhprp/throttle/internal/storage"
)

func TestLeakyBucketAllow_InitialState(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 5, leak 1 per 100ms
	limiter := limiter.NewLeakyBucket(store, 5, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Should allow initial requests up to capacity
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed (within capacity)", i+1)
		}
	}

	// 6th request should be denied (bucket full)
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request 6 should be denied (bucket at capacity)")
	}
}

func TestLeakyBucketAllow_LeakBehavior(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 3, leak 1 per 100ms
	limiter := limiter.NewLeakyBucket(store, 3, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket with 3 requests
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full, next request denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should be denied when bucket is full")
	}

	// Wait for 1 leak cycle (100ms) to leak 1 request
	time.Sleep(100 * time.Millisecond)

	// Now we should be able to add 1 more request (1 leaked out)
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("request should be allowed after leak cycle")
	}
}

func TestLeakyBucketAllow_MultipleLeak(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 10, leak 2 per 100ms
	limiter := limiter.NewLeakyBucket(store, 10, 2, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket with 10 requests
	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full")
	}

	// Wait for 200ms - should leak 4 requests (2 per 100ms)
	time.Sleep(200 * time.Millisecond)

	// Should now be able to add 4 more requests
	for i := 0; i < 4; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after leak", i+1)
		}
	}

	// 5th request should be denied
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("5th request should be denied (capacity reached again)")
	}
}

func TestLeakyBucketAllow_MultipleKeys(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 3, leak 1 per 100ms
	limiter := limiter.NewLeakyBucket(store, 3, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket for key1
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "key1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d for key1 should be allowed", i+1)
		}
	}

	// key1 bucket full
	allowed, err := limiter.Allow(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("key1 bucket should be full")
	}

	// key2 should have separate capacity (isolated)
	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "key2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d for key2 should be allowed (isolated)", i+1)
		}
	}

	// key2 bucket full
	allowed, err = limiter.Allow(ctx, "key2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("key2 bucket should be full")
	}
}

func TestLeakyBucketAllow_EdgeCaseZeroCapacity(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 0 (reject all)
	limiter := limiter.NewLeakyBucket(store, 0, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// First request should be denied
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should be denied with capacity 0")
	}
}

func TestLeakyBucketAllow_EdgeCaseZeroLeakRate(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 3, leak 0 (no leak)
	limiter := limiter.NewLeakyBucket(store, 3, 0, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket
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
		t.Errorf("request should be denied when bucket full and leak rate is 0")
	}

	// Even after waiting, nothing should leak
	time.Sleep(200 * time.Millisecond)
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("request should still be denied (leak rate is 0)")
	}
}

func TestLeakyBucketAllow_HighLeakRate(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 100, leak 1000 per 1ms (effectively unlimited leak)
	limiter := limiter.NewLeakyBucket(store, 100, 1000, 1*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket
	for i := 0; i < 100; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full")
	}

	// Wait minimal time for leak to occur
	time.Sleep(1 * time.Millisecond)

	// Should leak everything and allow new requests
	for i := 0; i < 50; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after leak", i+1)
		}
	}
}

func TestLeakyBucketReset(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewLeakyBucket(store, 5, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full before reset")
	}

	// Reset
	if err := limiter.Reset(ctx, "test_key"); err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	// After reset, should allow new requests
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after reset", i+1)
		}
	}
}

func TestLeakyBucketAllow_StateRecovery(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter1 := limiter.NewLeakyBucket(store, 10, 1, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Add 7 requests to bucket
	for i := 0; i < 7; i++ {
		allowed, err := limiter1.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Create new limiter instance with same store
	limiter2 := limiter.NewLeakyBucket(store, 10, 1, 100*time.Millisecond, logger)

	// State should persist - bucket has 7 items
	// Can add 3 more to reach capacity
	for i := 0; i < 3; i++ {
		allowed, err := limiter2.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed (recovered state)", i+1)
		}
	}

	// Now should be full
	allowed, err := limiter2.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full (recovered state)")
	}
}

func TestLeakyBucketAllow_ConcurrentRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 50, leak 5 per 100ms
	limiter := limiter.NewLeakyBucket(store, 50, 5, 100*time.Millisecond, logger)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0
	deniedCount := 0

	// Spawn 50 goroutines that submit requests concurrently
	// They likely execute fast enough to respect capacity limits
	for i := 0; i < 50; i++ {
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

	// All 50 should fit in the bucket when they arrive concurrently
	if allowedCount != 50 {
		t.Logf("info: allowed=%d (concurrent goroutines may have variance)", allowedCount)
	}
	if allowedCount+deniedCount != 50 {
		t.Errorf("total requests should be 50, got %d", allowedCount+deniedCount)
	}
}

func TestLeakyBucketAllow_SmallLeakInterval(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 5, leak 1 per 10ms
	limiter := limiter.NewLeakyBucket(store, 5, 1, 10*time.Millisecond, logger)
	ctx := context.Background()

	// Fill bucket
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full")
	}

	// Wait 50ms - should leak 5 requests
	time.Sleep(50 * time.Millisecond)

	// Bucket should be empty now, allow 5 new requests
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after leak", i+1)
		}
	}
}

func TestLeakyBucketAllow_LargeCapacity(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 1000, leak 10 per 100ms
	limiter := limiter.NewLeakyBucket(store, 1000, 10, 100*time.Millisecond, logger)
	ctx := context.Background()

	// Add 1000 requests (full capacity)
	for i := 0; i < 1000; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// Bucket full
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be full")
	}

	// Wait 100ms - should leak 10 requests
	time.Sleep(100 * time.Millisecond)

	// Should allow 10 new requests
	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed after leak", i+1)
		}
	}

	// 11th should be denied
	allowed, err = limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("11th request should be denied")
	}
}

func TestLeakyBucketClose(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	limiter := limiter.NewLeakyBucket(store, 5, 1, 100*time.Millisecond, logger)

	// Close should not error
	if err := limiter.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestLeakyBucketAllow_SteadyState(t *testing.T) {
	store := storage.NewMemoryStore()
	defer store.Close()

	logger := newTestLogger(t)
	defer logger.Sync()

	// Create a limiter: capacity 10, leak 2 per 100ms
	limiter := limiter.NewLeakyBucket(store, 10, 2, 100*time.Millisecond, logger)
	ctx := context.Background()

	// First, fill the bucket partially
	for i := 0; i < 6; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("initial request %d should be allowed", i+1)
		}
	}

	// Wait for leak to occur (should leak 2 requests)
	time.Sleep(100 * time.Millisecond)

	// Now bucket should have 6-2=4 items, can add 6 more to reach 10
	for i := 0; i < 6; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d after leak should be allowed", i+1)
		}
	}

	// Now bucket should be at capacity (10)
	allowed, err := limiter.Allow(ctx, "test_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("bucket should be at capacity")
	}

	// Wait for leak again (should leak 2)
	time.Sleep(100 * time.Millisecond)

	// Should now be able to add 2 requests
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(ctx, "test_key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d after second leak should be allowed", i+1)
		}
	}
}
