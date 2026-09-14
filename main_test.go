package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
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
	ts := httptest.NewServer(newRouter(rdb))
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

func writeTempConfig(t *testing.T, cfg Config) string {
	t.Helper()
	path := t.TempDir() + "/config.toml"
	content := "[server]\nport = " + strconv.Itoa(cfg.Server.Port) + "\n\n[redis]\naddr = \"" + cfg.Redis.Addr + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
