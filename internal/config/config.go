package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/netx"
)

const (
	defaultListen   = ":8080"
	defaultDataDir  = "./data"
	defaultLogLevel = "info"
)

type Config struct {
	Listen         string
	DataDir        string
	EmbyDir        string
	PublicURL      string
	STRMToken      string
	LogLevel       slog.Level
	AccessPassword string
	JWTSecret      string
	EmbyEnabled    bool
	EmbyServerURL  string
	EmbyAPIKey     string
	EmbyMediaPath  string
	EmbySyncActors bool
	TrustedProxies []string
}

// defaults are the built-in values of one startup mode. A container or
// environment-only start keeps the documented values; a portable Windows build
// keeps its data next to the executable instead.
type defaults struct {
	Listen  string
	DataDir string
}

func Load() (Config, error) {
	return resolve(nil, defaults{Listen: defaultListen, DataDir: defaultDataDir})
}

// resolve builds the configuration from the environment, then the optional
// portable configuration file, then the defaults of the startup mode.
func resolve(file map[string]string, fallback defaults) (Config, error) {
	source := settingSource{file: file}
	listen := source.value("MIYABI_LISTEN", fallback.Listen)
	dataDir := source.value("MIYABI_DATA_DIR", fallback.DataDir)
	logLevel, err := parseLogLevel(source.value("MIYABI_LOG_LEVEL", defaultLogLevel))
	if err != nil {
		return Config{}, err
	}
	embyDir := source.optional("MIYABI_EMBY_DIR")
	if embyDir == "" {
		embyDir = filepath.Join(dataDir, "emby")
	}
	publicURL := strings.TrimRight(source.optional("MIYABI_PUBLIC_URL"), "/")
	if publicURL == "" {
		port := "8080"
		if _, p, err := net.SplitHostPort(listen); err == nil && p != "" {
			port = p
		}
		publicURL = fmt.Sprintf("http://%s:%s", netx.OutboundIP(), port)
	}
	embyServerURL := strings.TrimRight(source.optional("MIYABI_EMBY_SERVER_URL"), "/")
	embyAPIKey := source.optional("MIYABI_EMBY_API_KEY")
	embyMediaPath := source.optional("MIYABI_EMBY_MEDIA_PATH")
	embyEnabledFlag := strings.ToLower(source.optional("MIYABI_EMBY_ENABLED"))
	embyEnabled := embyEnabledFlag == "true" || embyEnabledFlag == "1"
	if embyEnabledFlag == "" && (embyServerURL != "" || embyAPIKey != "") {
		embyEnabled = true
	}
	embySyncActorsFlag := strings.ToLower(source.optional("MIYABI_EMBY_SYNC_ACTORS"))
	embySyncActors := embySyncActorsFlag != "false" && embySyncActorsFlag != "0"

	cfg := Config{
		Listen:         listen,
		DataDir:        dataDir,
		EmbyDir:        embyDir,
		PublicURL:      publicURL,
		STRMToken:      source.optional("MIYABI_STRM_TOKEN"),
		LogLevel:       logLevel,
		AccessPassword: source.raw("MIYABI_ACCESS_PASSWORD"),
		JWTSecret:      source.optional("MIYABI_JWT_SECRET"),
		EmbyEnabled:    embyEnabled,
		EmbyServerURL:  embyServerURL,
		EmbyAPIKey:     embyAPIKey,
		EmbyMediaPath:  embyMediaPath,
		EmbySyncActors: embySyncActors,
		TrustedProxies: splitList(source.optional("MIYABI_TRUSTED_PROXIES")),
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// settingSource resolves one setting: an environment variable wins over the
// portable configuration file, which wins over the built-in defaults.
type settingSource struct {
	file map[string]string
}

// value returns a trimmed value or the fallback. A variable that is present but
// empty still counts as set, so an operator who clears MIYABI_LISTEN sees the
// validation error instead of a silent default.
func (source settingSource) value(key, fallback string) string {
	if raw, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(raw)
	}
	if value, ok := source.file[key]; ok {
		return value
	}
	return fallback
}

// optional reports an empty string for unset keys, which is how the keys whose
// empty value means "derive a default" are configured.
func (source settingSource) optional(key string) string {
	if raw, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(raw)
	}
	return source.file[key]
}

// raw preserves surrounding whitespace, which is significant for secrets.
func (source settingSource) raw(key string) string {
	if raw, ok := os.LookupEnv(key); ok {
		return raw
	}
	return source.file[key]
}

func splitList(value string) []string {
	if value == "" {
		return nil
	}
	var items []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func (cfg *Config) validate() error {
	if cfg.Listen == "" {
		return errors.New("MIYABI_LISTEN must not be empty")
	}
	if cfg.DataDir == "" {
		return errors.New("MIYABI_DATA_DIR must not be empty")
	}
	return nil
}

func parseLogLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("MIYABI_LOG_LEVEL must be debug, info, warn or error, got %q", name)
	}
}
