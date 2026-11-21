package catalogue

import (
	"context"
	"slices"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

const magnetSourcesSetting = "magnet.sources"

// MagnetSourceSetting is a user-facing on/off switch for one magnet source.
type MagnetSourceSetting struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// toggleableMagnetSources lists the sources the settings page may switch off.
// Other sources (e.g. JavBus, FreeJavBT) stay always on.
var toggleableMagnetSources = map[string]bool{
	domain.MagnetSourceSukebei:    true,
	domain.MagnetSourceProjectJav: true,
}

func magnetSourceOrder(id string) int {
	switch id {
	case domain.MagnetSourceSukebei:
		return 0
	case domain.MagnetSourceProjectJav:
		return 1
	default:
		return 2
	}
}

// applyMagnetToggles wraps the switchable sources so they can be toggled at
// runtime without rebuilding the aggregator.
func applyMagnetToggles(sources []magnet.Source) ([]magnet.Source, map[string]*magnet.Toggle, []MagnetSourceSetting) {
	out := make([]magnet.Source, 0, len(sources))
	toggles := make(map[string]*magnet.Toggle)
	var settings []MagnetSourceSetting
	for _, src := range sources {
		name := src.Name()
		if !toggleableMagnetSources[name] {
			out = append(out, src)
			continue
		}
		toggle := magnet.NewToggle(src, true)
		toggles[name] = toggle
		out = append(out, toggle)
		settings = append(settings, MagnetSourceSetting{ID: name, Enabled: true})
	}
	slices.SortStableFunc(settings, func(a, b MagnetSourceSetting) int {
		return magnetSourceOrder(a.ID) - magnetSourceOrder(b.ID)
	})
	return out, toggles, settings
}

// restoreMagnetSettings applies persisted switches, keeping newly registered
// sources at their default (enabled).
func (service *Service) restoreMagnetSettings(ctx context.Context) error {
	saved, found, err := database.LoadSetting[[]MagnetSourceSetting](ctx, service.database, magnetSourcesSetting)
	if err != nil || !found {
		return err
	}
	service.magnetMu.Lock()
	defer service.magnetMu.Unlock()
	states := make(map[string]bool, len(saved))
	for _, item := range saved {
		if toggle := service.magnetToggles[item.ID]; toggle != nil {
			toggle.SetEnabled(item.Enabled)
			states[item.ID] = item.Enabled
		}
	}
	for i := range service.magnetSettings {
		if enabled, ok := states[service.magnetSettings[i].ID]; ok {
			service.magnetSettings[i].Enabled = enabled
		}
	}
	return nil
}

// MagnetSourceSettings returns the user-switchable magnet sources.
func (service *Service) MagnetSourceSettings() []MagnetSourceSetting {
	service.magnetMu.RLock()
	defer service.magnetMu.RUnlock()
	return slices.Clone(service.magnetSettings)
}

// UpdateMagnetSourceSettings validates, persists and applies the switches.
func (service *Service) UpdateMagnetSourceSettings(ctx context.Context, settings []MagnetSourceSetting) error {
	service.magnetMu.Lock()
	defer service.magnetMu.Unlock()
	if len(settings) != len(service.magnetToggles) {
		return domain.E(domain.KindInvalid, "请提供完整的磁力来源列表", nil)
	}
	seen := make(map[string]bool, len(settings))
	for _, item := range settings {
		if service.magnetToggles[item.ID] == nil || seen[item.ID] {
			return domain.E(domain.KindInvalid, "未知或重复的磁力来源", nil)
		}
		seen[item.ID] = true
	}
	if err := database.SaveSetting(ctx, service.database, magnetSourcesSetting, settings); err != nil {
		return err
	}
	for _, item := range settings {
		service.magnetToggles[item.ID].SetEnabled(item.Enabled)
	}
	service.magnetSettings = slices.Clone(settings)
	return nil
}
