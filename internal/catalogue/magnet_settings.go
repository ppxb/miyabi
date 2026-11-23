package catalogue

import (
	"context"
	"slices"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

const magnetSourcesSetting = "magnet.sources"

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
func applyMagnetToggles(sources []magnet.Source) ([]magnet.Source, map[string]*magnet.Toggle, []domain.SourceToggle) {
	out := make([]magnet.Source, 0, len(sources))
	toggles := make(map[string]*magnet.Toggle)
	var settings []domain.SourceToggle
	for _, src := range sources {
		name := src.Name()
		if !toggleableMagnetSources[name] {
			out = append(out, src)
			continue
		}
		toggle := magnet.NewToggle(src, true)
		toggles[name] = toggle
		out = append(out, toggle)
		settings = append(settings, domain.SourceToggle{ID: name, Enabled: true})
	}
	slices.SortStableFunc(settings, func(a, b domain.SourceToggle) int {
		return magnetSourceOrder(a.ID) - magnetSourceOrder(b.ID)
	})
	return out, toggles, settings
}

// restoreMagnetSettings applies persisted switches, keeping newly registered
// sources at their default (enabled).
func (service *Service) restoreMagnetSettings(ctx context.Context) error {
	saved, found, err := database.LoadSetting[[]domain.SourceToggle](ctx, service.database, magnetSourcesSetting)
	if err != nil || !found {
		return err
	}
	service.magnetMu.Lock()
	defer service.magnetMu.Unlock()
	service.magnetSettings = domain.MergeSourceToggles(saved, service.magnetSettings)
	for _, item := range service.magnetSettings {
		service.magnetToggles[item.ID].SetEnabled(item.Enabled)
	}
	return nil
}

// MagnetSourceSettings returns the user-switchable magnet sources.
func (service *Service) MagnetSourceSettings() []domain.SourceToggle {
	service.magnetMu.RLock()
	defer service.magnetMu.RUnlock()
	return slices.Clone(service.magnetSettings)
}

// UpdateMagnetSourceSettings validates, persists and applies the switches.
func (service *Service) UpdateMagnetSourceSettings(ctx context.Context, settings []domain.SourceToggle) error {
	service.magnetMu.Lock()
	defer service.magnetMu.Unlock()
	if err := domain.ValidateSourceToggles(settings, service.magnetToggles, "磁力来源"); err != nil {
		return err
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
