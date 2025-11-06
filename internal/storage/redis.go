package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements Store interface using Redis
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new Redis store
func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: 10,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

// NewRedisStoreWithClient creates a new Redis store with an existing client
func NewRedisStoreWithClient(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// Increment increments the counter for the given key
func (s *RedisStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	pipe := s.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, expiration)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to increment key: %w", err)
	}

	return incr.Val(), nil
}

// IncrementBy increments the counter by a specific amount
func (s *RedisStore) IncrementBy(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error) {
	pipe := s.client.Pipeline()
	incrBy := pipe.IncrBy(ctx, key, value)
	pipe.Expire(ctx, key, expiration)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to increment key by value: %w", err)
	}

	return incrBy.Val(), nil
}

// Get retrieves the current value for the given key
func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get string key: %w", err)
	}
	return val, nil
}

// Set sets the value for the given key with expiration
func (s *RedisStore) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if err := s.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return fmt.Errorf("failed to set string key: %w", err)
	}
	return nil
}

// Delete removes the key from storage
func (s *RedisStore) Delete(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

// ZAdd adds a member with score to a sorted set
func (s *RedisStore) ZAdd(ctx context.Context, key string, score float64, member string, expiration time.Duration) error {
	pipe := s.client.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: member,
	})
	pipe.Expire(ctx, key, expiration)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to zadd: %w", err)
	}
	return nil
}

// ZRemRangeByScore removes members with scores in the given range
func (s *RedisStore) ZRemRangeByScore(ctx context.Context, key string, min, max float64) error {
	if err := s.client.ZRemRangeByScore(ctx, key, fmt.Sprintf("%f", min), fmt.Sprintf("%f", max)).Err(); err != nil {
		return fmt.Errorf("failed to zremrangebyscore: %w", err)
	}
	return nil
}

// ZCount counts members with scores in the given range
func (s *RedisStore) ZCount(ctx context.Context, key string, min, max float64) (int64, error) {
	count, err := s.client.ZCount(ctx, key, fmt.Sprintf("%f", min), fmt.Sprintf("%f", max)).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to zcount: %w", err)
	}
	return count, nil
}

// Expire sets expiration for a key
func (s *RedisStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if err := s.client.Expire(ctx, key, expiration).Err(); err != nil {
		return fmt.Errorf("failed to expire key: %w", err)
	}
	return nil
}

// Ping checks if the storage is accessible
func (s *RedisStore) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

// Close closes the storage connection
func (s *RedisStore) Close() error {
	if err := s.client.Close(); err != nil {
		return fmt.Errorf("failed to close redis connection: %w", err)
	}
	return nil
}

// GetClient returns the underlying Redis client (useful for advanced operations)
func (s *RedisStore) GetClient() *redis.Client {
	return s.client
}
