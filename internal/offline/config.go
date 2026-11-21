package offline

import (
	"context"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain/download"
	"github.com/ppxb/miyabi/internal/magnet"
)

func (s *Service) config(ctx context.Context) (download.Config, magnet.Preferences, error) {
	// Read only our fields so monitor and offline do not depend on each other.
	value, _, err := database.LoadSetting[struct {
		Download    *download.Config   `json:"download"`
		Preferences magnet.Preferences `json:"preferences"`
	}](ctx, s.database, "subscription.config")
	cfg := download.DefaultConfig()
	if value.Download != nil {
		cfg = *value.Download
	}
	prefs := value.Preferences.Normalized()
	if err == nil {
		err = prefs.Validate()
	}
	return cfg, prefs, err
}
