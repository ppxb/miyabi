package config

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/netx"
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

func Load() (Config, error) {
	listen := envOrDefault("MIYABI_LISTEN", ":8080")
	dataDir := envOrDefault("MIYABI_DATA_DIR", defaultDataDir())
	logLevel, err := parseLogLevel(envOrDefault("MIYABI_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	embyDir := strings.TrimSpace(os.Getenv("MIYABI_EMBY_DIR"))
	if embyDir == "" {
		embyDir = filepath.Join(dataDir, "emby")
	}
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MIYABI_PUBLIC_URL")), "/")
	if publicURL == "" {
		port := "8080"
		if _, p, err := net.SplitHostPort(listen); err == nil && p != "" {
			port = p
		}
		publicURL = fmt.Sprintf("http://%s:%s", netx.OutboundIP(), port)
	}
	embyServerURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MIYABI_EMBY_SERVER_URL")), "/")
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

	accessPassword := os.Getenv("MIYABI_ACCESS_PASSWORD")
	strmToken := strings.TrimSpace(os.Getenv("MIYABI_STRM_TOKEN"))
	if strmToken == "" && accessPassword != "" {
		strmToken = deriveSTRMToken(accessPassword)
	}

	cfg := Config{
		Listen:         listen,
		DataDir:        dataDir,
		EmbyDir:        embyDir,
		PublicURL:      publicURL,
		STRMToken:      strmToken,
		LogLevel:       logLevel,
		AccessPassword: accessPassword,
		JWTSecret:      strings.TrimSpace(os.Getenv("MIYABI_JWT_SECRET")),
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

// strmTokenSalt keeps the derived STRM token unrelated to the derived JWT secret.
const strmTokenSalt = "miyabi-strm-token"

// deriveSTRMToken gives a protected instance a fixed STRM token, so Emby and
// external players, which cannot send the session cookie, can still stream.
// The token is written into .strm files, so it is derived one-way.
func deriveSTRMToken(password string) string {
	mac := hmac.New(sha256.New, []byte(strmTokenSalt))
	mac.Write([]byte(password))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

// LocalURL returns the loopback address of the web UI for the tray menu.
func (cfg Config) LocalURL() string {
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// defaultDataDir stores runtime data next to the executable so a portable,
// double-clicked binary keeps its state beside it, since the working directory
// is launcher-dependent. MIYABI_DATA_DIR still overrides this default.
func defaultDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "./data"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	// `go run`/`go test` build into a temp go-build dir; keep the old
	// CWD-relative default there so dev runs do not scatter data into ephemeral
	// folders that are removed on exit.
	if strings.Contains(dir, "go-build") {
		return "./data"
	}
	return filepath.Join(dir, "data")
}

func envOrDefault(key, fallback string) string {
	if value, set := os.LookupEnv(key); set {
		return strings.TrimSpace(value)
	}
	return fallback
}

func (cfg *Config) validate() error {
	if cfg.Listen == "" {
		return errors.New("MIYABI_LISTEN must not be empty")
	}
	if cfg.DataDir == "" {
		return errors.New("MIYABI_DATA_DIR must not be empty")
	}
	for _, proxy := range cfg.TrustedProxies {
		if err := validateTrustedProxy(proxy); err != nil {
			return err
		}
	}
	return nil
}

// validateTrustedProxy accepts a bare IP or a CIDR block, matching what gin's
// SetTrustedProxies can parse, so a bad entry fails startup instead of being
// silently half-applied at runtime.
func validateTrustedProxy(entry string) error {
	if _, _, err := net.ParseCIDR(entry); err == nil {
		return nil
	}
	if net.ParseIP(entry) != nil {
		return nil
	}
	return fmt.Errorf("MIYABI_TRUSTED_PROXIES contains invalid IP or CIDR %q", entry)
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
