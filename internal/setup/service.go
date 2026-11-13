package setup

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

// State records whether the first-run wizard finished and which sources were
// selected. Onboarding is mandatory, so there is deliberately no dismissed flag.
type State struct {
	Completed   bool       `json:"completed"`
	Sources     []string   `json:"sources,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Service struct {
	db    *ent.Client
	mu    sync.RWMutex
	state State
}

// New loads the setup state. A missing record always means onboarding has not
// been completed; no earlier-version compatibility is applied.
func New(ctx context.Context, db *ent.Client) (*Service, error) {
	service := &Service{db: db}
	saved, found, err := database.LoadSetting[State](ctx, db, domain.SettingSetupState)
	if err != nil {
		return nil, err
	}
	if found {
		service.state = saved
	}
	return service, nil
}

func (s *Service) Status() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyState(s.state)
}

func (s *Service) NeedsSetup() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.state.Completed
}

var allowedSources = map[string]bool{"local": true, "115": true}

// Complete stores the final source selection and marks the wizard finished.
func (s *Service) Complete(ctx context.Context, sources []string) error {
	if len(sources) == 0 {
		return domain.E(domain.KindInvalid, "请至少选择一个影片来源", nil)
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if !allowedSources[source] {
			return domain.E(domain.KindInvalid, "不支持的影片来源", nil)
		}
		seen[source] = true
	}
	normalized := make([]string, 0, len(seen))
	for _, source := range []string{"local", "115"} {
		if seen[source] {
			normalized = append(normalized, source)
		}
	}
	now := time.Now()
	next := State{Completed: true, Sources: normalized, CompletedAt: &now}
	if err := database.SaveSetting(ctx, s.db, domain.SettingSetupState, next); err != nil {
		return err
	}
	s.mu.Lock()
	s.state = next
	s.mu.Unlock()
	return nil
}

func (s *Service) LocalDirectory(ctx context.Context) (string, error) {
	dir, found, err := database.LoadSetting[string](ctx, s.db, domain.SettingLocalDirectory)
	if err != nil || !found {
		return "", err
	}
	return dir, nil
}

// SetLocalDirectory validates and persists the local media root.
func (s *Service) SetLocalDirectory(ctx context.Context, dir string) error {
	absolute, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return domain.E(domain.KindInvalid, "本地目录路径无效", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return domain.E(domain.KindInvalid, "无法访问本地目录，请确认路径存在", err)
	}
	if !info.IsDir() {
		return domain.E(domain.KindInvalid, "本地路径不是目录", nil)
	}
	return database.SaveSetting(ctx, s.db, domain.SettingLocalDirectory, absolute)
}

func copyState(state State) State {
	copied := state
	if state.Sources != nil {
		copied.Sources = append([]string(nil), state.Sources...)
	}
	if state.CompletedAt != nil {
		finished := *state.CompletedAt
		copied.CompletedAt = &finished
	}
	return copied
}
