package config

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	defaultConfigPath = "config.toml"
	envPrefix         = "MIYABI_"
)

type Config struct {
	Listen   string `koanf:"listen"`
	DataDir  string `koanf:"data_dir"`
	LogLevel string `koanf:"log_level"`
	Proxy    string `koanf:"proxy"`
}

type cliOptions struct {
	configPath string
	listen     string
	dataDir    string
	logLevel   string
	proxy      string
	set        map[string]bool
}

func Load(args []string) (Config, error) {
	opts, err := parseCLI(args)
	if err != nil {
		return Config{}, err
	}

	k := koanf.New(".")
	if err := k.Load(confmap.Provider(map[string]any{
		"listen":    ":8080",
		"data_dir":  "./data",
		"log_level": "info",
		"proxy":     "",
	}, "."), nil); err != nil {
		return Config{}, fmt.Errorf("load default config: %w", err)
	}

	if err := k.Load(file.Provider(opts.configPath), toml.Parser()); err != nil {
		if opts.set["config"] || !errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("load config file %q: %w", opts.configPath, err)
		}
	}

	if err := k.Load(env.Provider(envPrefix, ".", func(value string) string {
		return strings.ToLower(strings.TrimPrefix(value, envPrefix))
	}), nil); err != nil {
		return Config{}, fmt.Errorf("load environment config: %w", err)
	}

	overrides := make(map[string]any, 4)
	if opts.set["listen"] {
		overrides["listen"] = opts.listen
	}
	if opts.set["data-dir"] {
		overrides["data_dir"] = opts.dataDir
	}
	if opts.set["log-level"] {
		overrides["log_level"] = opts.logLevel
	}
	if opts.set["proxy"] {
		overrides["proxy"] = opts.proxy
	}
	if err := k.Load(confmap.Provider(overrides, "."), nil); err != nil {
		return Config{}, fmt.Errorf("load command-line config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parseCLI(args []string) (cliOptions, error) {
	opts := cliOptions{set: make(map[string]bool)}
	flags := flag.NewFlagSet("miyabi", flag.ContinueOnError)
	flags.StringVar(&opts.configPath, "config", defaultConfigPath, "path to TOML config file")
	flags.StringVar(&opts.listen, "listen", "", "HTTP listen address")
	flags.StringVar(&opts.dataDir, "data-dir", "", "runtime data directory")
	flags.StringVar(&opts.logLevel, "log-level", "", "log level: debug, info, warn, error")
	flags.StringVar(&opts.proxy, "proxy", "", "outbound HTTP proxy")
	if err := flags.Parse(args); err != nil {
		return cliOptions{}, fmt.Errorf("parse command line: %w", err)
	}
	if flags.NArg() != 0 {
		return cliOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	flags.Visit(func(item *flag.Flag) {
		opts.set[item.Name] = true
	})
	return opts, nil
}

func (cfg *Config) validate() error {
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))

	if cfg.Listen == "" {
		return errors.New("listen address is required")
	}
	if cfg.DataDir == "" {
		return errors.New("data directory is required")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("unsupported log level %q", cfg.LogLevel)
	}
}
