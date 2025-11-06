package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheService provides helpers for interacting with Redis as a cache backend.
type CacheService interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
	GetJSON(ctx context.Context, key string, dest any) (bool, error)
	Delete(ctx context.Context, key string) error
}

type redisCacheService struct {
	client redis.Cmdable
}

// NewRedisCacheService returns a CacheService backed by Redis.
func NewRedisCacheService(client redis.Cmdable) CacheService {
	return &redisCacheService{client: client}
}

func (s *redisCacheService) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisCacheService) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return s.Set(ctx, key, payload, ttl)
}

func (s *redisCacheService) Get(ctx context.Context, key string) ([]byte, bool, error) {
	result, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}

	return result, true, nil
}

func (s *redisCacheService) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	payload, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		return ok, err
	}

	if err := json.Unmarshal(payload, dest); err != nil {
		return false, err
	}

	return true, nil
}

func (s *redisCacheService) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}
