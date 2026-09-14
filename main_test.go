package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig("config.toml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("server.port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("redis.addr = %q, want %q", cfg.Redis.Addr, "localhost:6379")
	}
	if cfg.Redis.KeyPrefix != "readwright:item:" {
		t.Errorf("redis.key_prefix = %q, want %q", cfg.Redis.KeyPrefix, "readwright:item:")
	}
}

func TestLoadConfigRejectsInvalid(t *testing.T) {
	t.Run("invalid port", func(t *testing.T) {
		cfg := Config{Server: ServerConfig{Port: 0}, Redis: RedisConfig{Addr: "localhost:6379"}}
		if _, err := LoadConfig(writeTempConfig(t, cfg)); err == nil {
			t.Error("LoadConfig() error = nil, want error for invalid server.port")
		}
	})
	t.Run("missing redis addr", func(t *testing.T) {
		cfg := Config{Server: ServerConfig{Port: 8080}}
		if _, err := LoadConfig(writeTempConfig(t, cfg)); err == nil {
			t.Error("LoadConfig() error = nil, want error for missing redis.addr")
		}
	})
}

func TestRunFailsFastWhenRedisUnavailable(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Port: 18079},
		Redis:  RedisConfig{Addr: "127.0.0.1:1"}, // refused immediately
	}
	if err := run(cfg); err == nil {
		t.Fatal("run() error = nil, want error when Redis is unreachable")
	}
}

// TestServerStartsWhenRedisAvailable verifies via httptest that the service
// starts and serves requests when Redis is reachable (real Redis per spec).
func TestServerStartsWhenRedisAvailable(t *testing.T) {
	cfg, err := LoadConfig("config.toml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	rdb := newRedisClient(cfg.Redis)
	defer rdb.Close()
	if pingErr := rdb.Ping(context.Background()).Err(); pingErr != nil {
		t.Fatalf("Redis not reachable at %s (start it, e.g. `docker run -d -p 6379:6379 redis:7-alpine`): %v", cfg.Redis.Addr, pingErr)
	}

	gin.SetMode(gin.TestMode)
	ts := httptest.NewServer(newRouter(rdb, cfg.Redis.KeyPrefix))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	// No business routes yet (#3-#5): an unmatched route must still be served
	// by the framework, proving the server is up.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET / status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestPutItem exercises PUT /items/:id against a real Redis at the HTTP API
// seam. Keys under the configured prefix are cleaned before and after.
func TestPutItem(t *testing.T) {
	cfg, err := LoadConfig("config.toml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	rdb := newRedisClient(cfg.Redis)
	t.Cleanup(func() { rdb.Close() })
	ctx := context.Background()
	if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
		t.Fatalf("Redis not reachable at %s (start it, e.g. `docker run -d -p 6379:6379 redis:7-alpine`): %v", cfg.Redis.Addr, pingErr)
	}
	prefix := cfg.Redis.KeyPrefix
	flushByPrefix(t, rdb, prefix)
	t.Cleanup(func() { flushByPrefix(t, rdb, prefix) })

	gin.SetMode(gin.TestMode)
	router := newRouter(rdb, prefix)

	t.Run("stores new value and returns 204 with no body", func(t *testing.T) {
		const id = "3001"
		w := doPut(router, id, "hello world")

		if w.Code != http.StatusNoContent {
			t.Errorf("PUT status = %d, want %d", w.Code, http.StatusNoContent)
		}
		if w.Body.Len() != 0 {
			t.Errorf("PUT body = %q, want empty", w.Body.String())
		}
		key := prefix + id
		got, err := rdb.Get(ctx, key).Result()
		if err != nil {
			t.Fatalf("redis GET %q error = %v", key, err)
		}
		if got != "hello world" {
			t.Errorf("stored value = %q, want %q", got, "hello world")
		}
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			t.Fatalf("redis TTL %q error = %v", key, err)
		}
		if ttl != noExpiryTTL {
			t.Errorf("TTL of %q = %v, want %v (no expiry)", key, ttl, noExpiryTTL)
		}
	})

	t.Run("overwrites an existing value and returns 204", func(t *testing.T) {
		const id = "3002"
		key := prefix + id
		if w := doPut(router, id, "first value"); w.Code != http.StatusNoContent {
			t.Fatalf("first PUT status = %d, want %d", w.Code, http.StatusNoContent)
		}
		if w := doPut(router, id, "second value"); w.Code != http.StatusNoContent {
			t.Fatalf("second PUT status = %d, want %d", w.Code, http.StatusNoContent)
		}

		got, err := rdb.Get(ctx, key).Result()
		if err != nil {
			t.Fatalf("redis GET %q error = %v", key, err)
		}
		if got != "second value" {
			t.Errorf("stored value = %q, want %q", got, "second value")
		}
	})

	t.Run("rejects non-positive-integer ids with 400 and writes nothing", func(t *testing.T) {
		for _, badID := range []string{"abc", "0", "-1", "1.5"} {
			t.Run(badID, func(t *testing.T) {
				w := doPut(router, badID, "value")

				if w.Code != http.StatusBadRequest {
					t.Errorf("PUT /items/%s status = %d, want %d", badID, w.Code, http.StatusBadRequest)
				}
				if n, err := rdb.Exists(ctx, prefix+badID).Result(); err != nil {
					t.Fatalf("redis EXISTS %q error = %v", prefix+badID, err)
				} else if n != 0 {
					t.Errorf("key %q exists = %d, want it absent after rejected PUT", prefix+badID, n)
				}
			})
		}
	})

	t.Run("returns 500 when the Redis write fails", func(t *testing.T) {
		badRdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // refused immediately
		t.Cleanup(func() { badRdb.Close() })

		w := doPut(newRouter(badRdb, prefix), "3003", "value")

		if w.Code != http.StatusInternalServerError {
			t.Errorf("PUT status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

// noExpiryTTL is how go-redis reports Redis' "no expiry" TTL (-1).
const noExpiryTTL = -time.Nanosecond

// doPut sends PUT /items/:id with a text/plain body through h and returns the
// recorded response.
func doPut(h http.Handler, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/items/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// flushByPrefix deletes every key under prefix so tests start and end clean.
func flushByPrefix(t *testing.T, rdb *redis.Client, prefix string) {
	t.Helper()
	ctx := context.Background()
	keys, err := rdb.Keys(ctx, prefix+"*").Result()
	if err != nil {
		t.Fatalf("list keys under %q: %v", prefix+"*", err)
	}
	if len(keys) > 0 {
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			t.Fatalf("delete keys %v: %v", keys, err)
		}
	}
}

func writeTempConfig(t *testing.T, cfg Config) string {
	t.Helper()
	path := t.TempDir() + "/config.toml"
	content := "[server]\nport = " + strconv.Itoa(cfg.Server.Port) + "\n\n[redis]\naddr = \"" + cfg.Redis.Addr + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
