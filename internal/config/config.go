package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Listen         string
	DataDir        string
	LogLevel       string
	Proxy          string
	AccessPassword string
}

func Load() (Config, error) {
	cfg := Config{
		Listen:         envOrDefault("MIYABI_LISTEN", ":8080"),
		DataDir:        envOrDefault("MIYABI_DATA_DIR", "./data"),
		LogLevel:       envOrDefault("MIYABI_LOG_LEVEL", "info"),
		Proxy:          os.Getenv("MIYABI_PROXY"),
		AccessPassword: os.Getenv("MIYABI_ACCESS_PASSWORD"),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value, set := os.LookupEnv(key); set {
		return value
	}
	return fallback
}

func (cfg *Config) validate() error {
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	if cfg.Listen == "" {
		return errors.New("MIYABI_LISTEN must not be empty")
	}
	if cfg.DataDir == "" {
		return errors.New("MIYABI_DATA_DIR must not be empty")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("MIYABI_LOG_LEVEL must be debug, info, warn or error, got %q", cfg.LogLevel)
	}
}
