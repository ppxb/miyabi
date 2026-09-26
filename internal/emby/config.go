package emby

import (
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
)

const SettingKey = "emby.config"

// Config represents Emby integration settings.
type Config struct {
	Enabled    bool   `json:"enabled"`
	ServerURL  string `json:"server_url"`
	APIKey     string `json:"api_key"`
	MediaPath  string `json:"media_path"`
	LocalDir   string `json:"local_dir"`
	SyncActors *bool  `json:"sync_actors,omitempty"`
	PublicURL  string `json:"public_url,omitempty"`
}

// IsSyncActors returns whether actor avatar synchronization is enabled (defaults to true).
func (c *Config) IsSyncActors() bool {
	if c.SyncActors == nil {
		return true
	}
	return *c.SyncActors
}

// Normalize trims inputs and validates required fields when enabled.
func (c *Config) Normalize() error {
	c.ServerURL = strings.TrimRight(strings.TrimSpace(c.ServerURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.MediaPath = strings.TrimSpace(c.MediaPath)
	c.LocalDir = strings.TrimSpace(c.LocalDir)
	c.PublicURL = strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")
	if c.SyncActors == nil {
		defaultSync := true
		c.SyncActors = &defaultSync
	}

	if c.Enabled {
		if c.ServerURL == "" {
			return domain.E(domain.KindInvalid, "启用 Emby 时必须填写服务器地址", nil)
		}
		u, err := url.Parse(c.ServerURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return domain.E(domain.KindInvalid, "Emby 服务器地址格式不正确，需包含 http:// 或 https://", nil)
		}
		if c.APIKey == "" {
			return domain.E(domain.KindInvalid, "启用 Emby 时必须填写 API Key", nil)
		}
	}
	return nil
}
