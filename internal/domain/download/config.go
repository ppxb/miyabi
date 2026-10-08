package download

import (
	"encoding/json"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

// Config is shared by subscription selection and offline recovery.
type Config struct {
	AutoSwitch             bool `json:"auto_switch"`
	ZeroProgressMinutes    int  `json:"zero_progress_minutes"`
	StalledMinutes         int  `json:"stalled_minutes"`
	CompletionGraceMinutes int  `json:"completion_grace_minutes"`
	MaxAttempts            int  `json:"max_attempts"`
}

func DefaultConfig() Config {
	return Config{AutoSwitch: true, ZeroProgressMinutes: 15, StalledMinutes: 30, CompletionGraceMinutes: 60, MaxAttempts: 3}
}

func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	value := plain(DefaultConfig())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = Config(value)
	return nil
}

func (c Config) Validate() error {
	for _, minutes := range []int{c.ZeroProgressMinutes, c.StalledMinutes, c.CompletionGraceMinutes} {
		if minutes < 1 || minutes > 1440 {
			return domain.E(domain.KindInvalid, "无进展等待时间应为 1–1440 分钟", nil)
		}
	}
	if c.MaxAttempts < 1 || c.MaxAttempts > 10 {
		return domain.E(domain.KindInvalid, "磁力尝试上限应为 1–10 条", nil)
	}
	return nil
}

func (c Config) Timeout(progress float64) time.Duration {
	minutes := c.StalledMinutes
	if progress == 0 {
		minutes = c.ZeroProgressMinutes
	} else if progress >= 95 {
		minutes = max(minutes, c.CompletionGraceMinutes)
	}
	return time.Duration(minutes) * time.Minute
}
