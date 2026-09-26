package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/logging"
	"github.com/ppxb/miyabi/internal/netx"
)

type Config struct {
	Listen         string
	DataDir        string
	EmbyDir        string
	PublicURL      string
	STRMToken      string
	LogLevel       string
	AccessPassword string
	JWTSecret      string
	Runtime        Runtime
	EmbyEnabled    bool
	EmbyServerURL  string
	EmbyAPIKey     string
	EmbyMediaPath  string
	EmbySyncActors bool
	TrustedProxies []string
}

func Load() (Config, error) {
	listen := envOrDefault("MIYABI_LISTEN", ":8080")
	dataDir := envOrDefault("MIYABI_DATA_DIR", "./data")
	embyDir := strings.TrimSpace(os.Getenv("MIYABI_EMBY_DIR"))
	if embyDir == "" {
		embyDir = filepath.Join(dataDir, "emby")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MIYABI_PUBLIC_URL")), "/")
	if publicURL == "" {
		port := "8080"
		if _, p, err := net.SplitHostPort(strings.TrimSpace(listen)); err == nil && p != "" {
			port = p
		}
		publicURL = fmt.Sprintf("http://%s:%s", netx.OutboundIP(), port)
	}
	embyServerURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MIYABI_EMBY_SERVER_URL")), "/")
	if embyServerURL == "" {
		embyServerURL = strings.TrimRight(strings.TrimSpace(os.Getenv("MIYABI_EMBY_URL")), "/")
	}
	embyAPIKey := strings.TrimSpace(os.Getenv("MIYABI_EMBY_API_KEY"))
	embyMediaPath := strings.TrimSpace(os.Getenv("MIYABI_EMBY_MEDIA_PATH"))
	embyEnabledStr := strings.ToLower(strings.TrimSpace(os.Getenv("MIYABI_EMBY_ENABLED")))
	embyEnabled := embyEnabledStr == "true" || embyEnabledStr == "1"
	if embyEnabledStr == "" && (embyServerURL != "" || embyAPIKey != "") {
		embyEnabled = true
	}
	embySyncActorsStr := strings.ToLower(strings.TrimSpace(os.Getenv("MIYABI_EMBY_SYNC_ACTORS")))
	embySyncActors := embySyncActorsStr != "false" && embySyncActorsStr != "0"

	var trustedProxies []string
	if rawProxies := strings.TrimSpace(os.Getenv("MIYABI_TRUSTED_PROXIES")); rawProxies != "" {
		for _, p := range strings.Split(rawProxies, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				trustedProxies = append(trustedProxies, trimmed)
			}
		}
	}

	cfg := Config{
		Listen:         listen,
		DataDir:        dataDir,
		EmbyDir:        embyDir,
		PublicURL:      publicURL,
		STRMToken:      strings.TrimSpace(os.Getenv("MIYABI_STRM_TOKEN")),
		LogLevel:       envOrDefault("MIYABI_LOG_LEVEL", "info"),
		AccessPassword: os.Getenv("MIYABI_ACCESS_PASSWORD"),
		JWTSecret:      strings.TrimSpace(os.Getenv("MIYABI_JWT_SECRET")),
		Runtime:        DefaultRuntime(),
		EmbyEnabled:    embyEnabled,
		EmbyServerURL:  embyServerURL,
		EmbyAPIKey:     embyAPIKey,
		EmbyMediaPath:  embyMediaPath,
		EmbySyncActors: embySyncActors,
		TrustedProxies: trustedProxies,
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
	cfg.EmbyDir = strings.TrimSpace(cfg.EmbyDir)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	if cfg.Listen == "" {
		return errors.New("MIYABI_LISTEN must not be empty")
	}
	if cfg.DataDir == "" {
		return errors.New("MIYABI_DATA_DIR must not be empty")
	}
	if cfg.EmbyDir == "" {
		return errors.New("MIYABI_EMBY_DIR must not be empty")
	}
	if err := logging.Validate(cfg.LogLevel); err != nil {
		return fmt.Errorf("MIYABI_LOG_LEVEL %w", err)
	}
	return nil
}
