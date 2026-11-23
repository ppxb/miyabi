package domain

// SourceToggle is a user-facing on/off switch for one data source.
type SourceToggle struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// ValidateSourceToggles checks that settings names every known source exactly
// once. label names the source family in error messages, e.g. "刮削来源".
func ValidateSourceToggles[V any](settings []SourceToggle, known map[string]V, label string) error {
	if len(settings) != len(known) {
		return E(KindInvalid, "请提供完整的"+label+"列表", nil)
	}
	seen := make(map[string]bool, len(settings))
	for _, item := range settings {
		if _, ok := known[item.ID]; !ok || seen[item.ID] {
			return E(KindInvalid, "未知或重复的"+label, nil)
		}
		seen[item.ID] = true
	}
	return nil
}

// MergeSourceToggles keeps the saved switches of registered sources in their
// saved order, drops sources no longer registered, and appends newly
// registered ones with their registered defaults.
func MergeSourceToggles(saved, registered []SourceToggle) []SourceToggle {
	known := make(map[string]bool, len(registered))
	for _, item := range registered {
		known[item.ID] = true
	}
	out := make([]SourceToggle, 0, len(registered))
	kept := make(map[string]bool, len(saved))
	for _, item := range saved {
		if known[item.ID] && !kept[item.ID] {
			out = append(out, item)
			kept[item.ID] = true
		}
	}
	for _, item := range registered {
		if !kept[item.ID] {
			out = append(out, item)
		}
	}
	return out
}
