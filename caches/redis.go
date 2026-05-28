package caches

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ArthurHlt/rparth/models"
	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "rparth:"

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(url string, ttl time.Duration) (*RedisCache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisCache{client: client, ttl: ttl}, nil
}

func (r *RedisCache) Get(key string) (*models.CacheData, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	payload, err := r.client.Get(ctx, redisKeyPrefix+key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("redis cache get failed", "err", err, "key", key)
		}
		return nil, false
	}
	var data models.CacheData
	if err := json.Unmarshal(payload, &data); err != nil {
		slog.Warn("redis cache decode failed", "err", err, "key", key)
		return nil, false
	}
	return &data, true
}

func (r *RedisCache) Set(key string, data *models.CacheData) {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Warn("redis cache encode failed", "err", err, "key", key)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.client.Set(ctx, redisKeyPrefix+key, payload, r.ttl).Err(); err != nil {
		slog.Warn("redis cache set failed", "err", err, "key", key)
	}
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}

func (r *RedisCache) Len() int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var (
		count  int
		cursor uint64
	)
	for {
		keys, next, err := r.client.Scan(ctx, cursor, redisKeyPrefix+"*", 100).Result()
		if err != nil {
			slog.Warn("redis cache len failed", "err", err)
			return 0
		}
		count += len(keys)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return count
}
