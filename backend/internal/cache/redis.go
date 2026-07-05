package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
	return &RedisCache{client: redis.NewClient(&redis.Options{Addr: addr})}
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, bool) {
	if c == nil || c.client == nil {
		return "", false
	}
	value, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return value, true
}

func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) {
	if c == nil || c.client == nil {
		return
	}
	_ = c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
