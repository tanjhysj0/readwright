package main

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server ServerConfig `toml:"server"`
	Redis  RedisConfig  `toml:"redis"`
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type RedisConfig struct {
	Addr      string `toml:"addr"`
	Password  string `toml:"password"`
	DB        int    `toml:"db"`
	KeyPrefix string `toml:"key_prefix"`
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return Config{}, fmt.Errorf("invalid server.port: %d", cfg.Server.Port)
	}
	if cfg.Redis.Addr == "" {
		return Config{}, fmt.Errorf("redis.addr is required")
	}
	return cfg, nil
}
