package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
)

func TestMemoryStoreIncrement(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Test first increment
	val, err := ms.Increment(ctx, "counter", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Test second increment
	val, err = ms.Increment(ctx, "counter", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}
}

func TestMemoryStoreIncrementBy(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Test increment by 5
	val, err := ms.IncrementBy(ctx, "counter", 5, 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	// Test increment by 3
	val, err = ms.IncrementBy(ctx, "counter", 3, 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 8 {
		t.Errorf("expected 8, got %d", val)
	}
}

func TestMemoryStoreGetSet(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Test set and get
	err := ms.Set(ctx, "key", "value", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := ms.Get(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %s", val)
	}

	// Test get non-existent key
	val, err = ms.Get(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}
}

func TestMemoryStoreDelete(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Set and delete
	ms.Set(ctx, "key", "value", 1*time.Minute)
	err := ms.Delete(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err := ms.Get(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string after delete, got %s", val)
	}
}

func TestMemoryStoreExpiration(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Set value with short expiration
	err := ms.Set(ctx, "expiring_key", "value", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exist immediately
	val, err := ms.Get(ctx, "expiring_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %s", val)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not exist after expiration
	val, err = ms.Get(ctx, "expiring_key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string after expiration, got %s", val)
	}
}

func TestMemoryStoreZAdd(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Add members to sorted set
	err := ms.ZAdd(ctx, "zset", 1.0, "member1", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = ms.ZAdd(ctx, "zset", 2.0, "member2", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count should work
	count, err := ms.ZCount(ctx, "zset", 0.0, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestMemoryStoreZCount(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Add members with different scores
	ms.ZAdd(ctx, "zset", 1.0, "m1", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 2.0, "m2", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 3.0, "m3", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 4.0, "m4", 1*time.Minute)

	// Count members with score between 1.5 and 3.5
	count, err := ms.ZCount(ctx, "zset", 1.5, 3.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Count non-existent sorted set
	count, err = ms.ZCount(ctx, "non-existent", 0.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestMemoryStoreZRemRangeByScore(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Add members with different scores
	ms.ZAdd(ctx, "zset", 1.0, "m1", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 2.0, "m2", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 3.0, "m3", 1*time.Minute)
	ms.ZAdd(ctx, "zset", 4.0, "m4", 1*time.Minute)

	// Remove members with score between 1.5 and 3.5
	err := ms.ZRemRangeByScore(ctx, "zset", 1.5, 3.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count remaining members
	count, err := ms.ZCount(ctx, "zset", 0.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2 (remaining members), got %d", count)
	}
}

func TestMemoryStoreExpire(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Set a key with long expiration
	ms.Set(ctx, "key", "value", 10*time.Second)

	// Update expiration to short duration
	err := ms.Expire(ctx, "key", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exist immediately
	val, err := ms.Get(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "value" {
		t.Errorf("expected 'value', got %s", val)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	val, err = ms.Get(ctx, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string after expiration, got %s", val)
	}
}

func TestMemoryStorePing(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	err := ms.Ping(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemoryStoreClose(t *testing.T) {
	ms := storage.NewMemoryStore()

	err := ms.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMemoryStoreConcurrency(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()
	done := make(chan bool)

	// Concurrent increments
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ms.Increment(ctx, "concurrent", 1*time.Minute)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check final value
	val, err := ms.Get(ctx, "concurrent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var finalVal int64
	if _, err := fmt.Sscanf(val, "%d", &finalVal); err != nil {
		t.Fatalf("unexpected error parsing value: %v", err)
	}

	if finalVal != 1000 {
		t.Errorf("expected 1000, got %d", finalVal)
	}
}

func TestMemoryStoreZSetExpiration(t *testing.T) {
	ms := storage.NewMemoryStore()
	defer ms.Close()

	ctx := context.Background()

	// Add to sorted set with short expiration
	err := ms.ZAdd(ctx, "zset", 1.0, "member", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exist immediately
	count, err := ms.ZCount(ctx, "zset", 0.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	count, err = ms.ZCount(ctx, "zset", 0.0, 10.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after expiration, got %d", count)
	}
}
