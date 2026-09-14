package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

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

func newRouter(rdb *redis.Client, keyPrefix string) *gin.Engine {
	r := gin.Default()
	r.PUT("/items/:id", putItem(rdb, keyPrefix))
	return r
}

// putItem handles PUT /items/:id: it stores the plain-text request body at
// {keyPrefix}{id} in Redis with no TTL, overwriting any existing value.
func putItem(rdb *redis.Client, keyPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.String(http.StatusBadRequest, "id must be a positive integer\n")
			return
		}
		value, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusBadRequest, "failed to read request body\n")
			return
		}
		key := keyPrefix + strconv.FormatInt(id, 10)
		if err := rdb.Set(c.Request.Context(), key, value, 0).Err(); err != nil {
			c.String(http.StatusInternalServerError, "redis set failed\n")
			return
		}
		c.Status(http.StatusNoContent)
	}
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
	return newRouter(rdb, cfg.Redis.KeyPrefix).Run(addr)
}
