package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	*redis.Client
}

func initRedis() *RedisClient {
	addr := os.Getenv("REDIS_URL")
	if addr == "" {
		panic("NEED TO SET REDIS_URL ENV VAR")
	}

	opt, _ := redis.ParseURL(addr)
	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		panic("Failed to connect to Redis: " + err.Error())
	}

	appLogger.Info("Connected to Redis successfully")
	return &RedisClient{Client: client}
}

func (r *RedisClient) getShareCache(userID string) ([]Upload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	key := fmt.Sprintf("user:shares:%s", userID)
	val, err := r.Get(ctx, key).Result()

	if err == redis.Nil {
		return nil, err
	}
	if err != nil {
		appLogger.WithError(err).Warn("Redis failed")
		return nil, err
	}

	var uploads []Upload
	if err := json.Unmarshal([]byte(val), &uploads); err != nil {
		appLogger.WithError(err).Error("Failed to unmarshal cached shares")
		return nil, err
	}

	appLogger.WithField("user_id", userID).Debug("Cache hit for shares")
	return uploads, nil
}

func (r *RedisClient) setShareCache(userID string, uploads []Upload) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	data, err := json.Marshal(uploads)
	if err != nil {
		appLogger.WithError(err).Error("Failed to marshal shares for cache")
		return
	}

	key := fmt.Sprintf("user:shares:%s", userID)
	ttl := 48 * time.Hour

	if err := r.Set(ctx, key, data, ttl).Err(); err != nil {
		appLogger.WithError(err).Warn("Failed to cache shares")
	} else {
		appLogger.WithField("user_id", userID).Debug("Shares cached successfully")
	}
}

func (r *RedisClient) deleteShareCache(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	key := fmt.Sprintf("user:shares:%s", userID)
	if err := r.Del(ctx, key).Err(); err != nil {
		appLogger.WithError(err).Warn("Failed to delete share cache")
	} else {
		appLogger.WithField("user_id", userID).Debug("Share cache deleted successfully")
	}
}
