// Package localsource manages the local media directories that Miyabi scans
// alongside the mounted 115 library.
package localsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

// settingKey stores every configured local media directory in one settings row.
const settingKey = "local.sources"

// Source is one configured local media directory.
type Source struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// Update describes the mutable fields of a configured source.
type Update struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

// Manager reads and writes the configured local media directories.
type Manager struct {
	database *ent.Client
}

func NewManager(database *ent.Client) *Manager {
	return &Manager{database: database}
}

// List returns every configured directory in the order it was added.
func (m *Manager) List(ctx context.Context) ([]Source, error) {
	sources, _, err := database.LoadSetting[[]Source](ctx, m.database, settingKey)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		return []Source{}, nil
	}
	return sources, nil
}

// Find returns one configured directory; the second result reports whether the ID exists.
func (m *Manager) Find(ctx context.Context, id string) (Source, bool, error) {
	sources, err := m.List(ctx)
	if err != nil {
		return Source{}, false, err
	}
	for _, source := range sources {
		if source.ID == id {
			return source, true, nil
		}
	}
	return Source{}, false, nil
}

// Add validates a directory and stores it as an enabled source.
func (m *Manager) Add(ctx context.Context, name, path string) (Source, error) {
	resolved, err := resolve(path)
	if err != nil {
		return Source{}, err
	}
	sources, err := m.List(ctx)
	if err != nil {
		return Source{}, err
	}
	source := Source{ID: identity(resolved), Name: strings.TrimSpace(name), Path: resolved, Enabled: true}
	if source.Name == "" {
		source.Name = filepath.Base(resolved)
	}
	for _, existing := range sources {
		// The identity covers symlinks and relative spellings, so the same
		// directory cannot be added twice under different paths.
		if existing.ID == source.ID {
			return Source{}, domain.E(domain.KindConflict, "该目录已添加为本地媒体目录", nil)
		}
	}
	if err := m.save(ctx, append(sources, source)); err != nil {
		return Source{}, err
	}
	return source, nil
}

// Update renames or enables and disables one source.
func (m *Manager) Update(ctx context.Context, id string, update Update) (Source, error) {
	sources, err := m.List(ctx)
	if err != nil {
		return Source{}, err
	}
	for index, source := range sources {
		if source.ID != id {
			continue
		}
		if update.Name != nil {
			name := strings.TrimSpace(*update.Name)
			if name == "" {
				return Source{}, domain.E(domain.KindInvalid, "请填写目录名称", nil)
			}
			source.Name = name
		}
		if update.Enabled != nil {
			source.Enabled = *update.Enabled
		}
		sources[index] = source
		if err := m.save(ctx, sources); err != nil {
			return Source{}, err
		}
		return source, nil
	}
	return Source{}, domain.E(domain.KindNotFound, "本地媒体目录不存在", nil)
}

// Remove deletes one source. Files already indexed from it stay in the library.
func (m *Manager) Remove(ctx context.Context, id string) error {
	sources, err := m.List(ctx)
	if err != nil {
		return err
	}
	kept := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.ID != id {
			kept = append(kept, source)
		}
	}
	if len(kept) == len(sources) {
		return domain.E(domain.KindNotFound, "本地媒体目录不存在", nil)
	}
	return m.save(ctx, kept)
}

func (m *Manager) save(ctx context.Context, sources []Source) error {
	return database.SaveSetting(ctx, m.database, settingKey, sources)
}

// resolve canonicalizes a user-supplied directory and rejects paths that are
// not readable directories.
func resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", domain.E(domain.KindInvalid, "请填写本地目录路径", nil)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", domain.E(domain.KindInvalid, "本地目录路径无效", err)
	}
	// Relative segments and symlinks must collapse to one canonical directory
	// so that a source keeps a single identity across configurations.
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", domain.E(domain.KindInvalid, "本地目录不存在或无法访问", err)
	}
	stat, err := os.Stat(canonical)
	if err != nil {
		return "", domain.E(domain.KindInvalid, "本地目录不存在或无法访问", err)
	}
	if !stat.IsDir() {
		return "", domain.E(domain.KindInvalid, "本地媒体目录必须是文件夹", nil)
	}
	return canonical, nil
}

// identity derives a stable ID from a canonical path, so renaming a source
// never re-identifies it or the files already indexed under it.
func identity(path string) string {
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	sum := sha256.Sum256([]byte(path))
	return "src-" + hex.EncodeToString(sum[:8])
}
