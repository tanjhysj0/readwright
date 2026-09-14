package main

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func newRedisClient(cfg RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func newRouter(rdb *redis.Client) *gin.Engine {
	// rdb is wired in for the business routes added by later tickets (#3-#5).
	r := gin.Default()
	return r
}

// run boots the service: create the Redis client, ping it (fail fast on error),
// then serve HTTP on the configured port until the listener fails.
func run(cfg Config) error {
	rdb := newRedisClient(cfg.Redis)
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	return newRouter(rdb).Run(addr)
}
