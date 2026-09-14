package main

import (
	"log"
)

func main() {
	cfg, err := LoadConfig("config.toml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := run(cfg); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
