package source

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
)

const settingKey = "sources"

// Kind identifies how a storage source is reached.
type Kind string

const (
	KindLocal  Kind = "local"
	Kind115    Kind = "115"
	KindPikPak Kind = "pikpak"
)

// Source is a configurable storage location (a local folder or a cloud drive).
type Source struct {
	ID      string            `json:"id"`
	Kind    Kind              `json:"kind"`
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config,omitempty"`
}

// Service persists the configured storage sources as a single settings row.
type Service struct {
	client      *ent.Client
	onLocalScan func(context.Context, string, bool) error
	onScan      func(context.Context, domain.LibrarySource) error
	mounts      MountProvider
}

// LocalTarget is an enabled local source directory and its metadata mode.
type LocalTarget struct {
	Path         string
	MetadataOnly bool
}

// MountProvider resolves a 115 account's mounted directory.
type MountProvider interface {
	Mount(context.Context, string) (domain.LibrarySource, bool, error)
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
}

// SetLocalScanHandler lets the composition root enqueue scans after sources change.
func (s *Service) SetLocalScanHandler(handler func(context.Context, string, bool) error) {
	s.onLocalScan = handler
}

// SetScanHandler enqueues scans for configured cloud sources.
func (s *Service) SetScanHandler(handler func(context.Context, domain.LibrarySource) error) {
	s.onScan = handler
}

// SetMountProvider lets the service resolve 115 account mounts.
func (s *Service) SetMountProvider(provider MountProvider) {
	s.mounts = provider
}

func (s *Service) List(ctx context.Context) ([]Source, error) {
	sources, _, err := database.LoadSetting[[]Source](ctx, s.client, settingKey)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = []Source{}
	}
	return sources, nil
}

func (s *Service) Replace(ctx context.Context, sources []Source) ([]Source, error) {
	if sources == nil {
		sources = []Source{}
	}
	for i := range sources {
		if strings.TrimSpace(sources[i].ID) == "" {
			sources[i].ID = uuid.NewString()
		}
		if err := validate(sources[i]); err != nil {
			return nil, err
		}
	}
	if err := database.SaveSetting(ctx, s.client, settingKey, sources); err != nil {
		return nil, err
	}
	s.triggerScans(ctx, sources)
	return sources, nil
}

func (s *Service) triggerScans(ctx context.Context, sources []Source) {
	for _, src := range sources {
		if !src.Enabled {
			continue
		}
		switch src.Kind {
		case KindLocal:
			if path := strings.TrimSpace(src.Config["path"]); path != "" && s.onLocalScan != nil {
				_ = s.onLocalScan(ctx, path, localMetadataOnly(src))
			}
		case Kind115:
			accountID := strings.TrimSpace(src.Config["account_id"])
			if accountID == "" || s.mounts == nil {
				continue
			}
			if mount, ok, err := s.mounts.Mount(ctx, accountID); err == nil && ok && s.onScan != nil {
				_ = s.onScan(ctx, mount)
			}
		}
	}
}

// LibrarySources returns the enabled 115 sources that have a mounted directory.
func (s *Service) LibrarySources(ctx context.Context) ([]domain.LibrarySource, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.LibrarySource, 0)
	for _, src := range sources {
		if src.Kind != Kind115 || !src.Enabled || s.mounts == nil {
			continue
		}
		accountID := strings.TrimSpace(src.Config["account_id"])
		if accountID == "" {
			continue
		}
		mount, ok, err := s.mounts.Mount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, mount)
		}
	}
	return result, nil
}

// LocalTargets returns the enabled local source directories with their mode.
func (s *Service) LocalTargets(ctx context.Context) ([]LocalTarget, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]LocalTarget, 0)
	for _, src := range sources {
		if src.Kind != KindLocal || !src.Enabled {
			continue
		}
		if path := strings.TrimSpace(src.Config["path"]); path != "" {
			targets = append(targets, LocalTarget{Path: path, MetadataOnly: localMetadataOnly(src)})
		}
	}
	return targets, nil
}

func localMetadataOnly(src Source) bool {
	return strings.TrimSpace(src.Config["scrape_mode"]) == "local"
}

// LocalRoots returns the enabled local source directories, for startup scanning.
func (s *Service) LocalRoots(ctx context.Context) ([]string, error) {
	targets, err := s.LocalTargets(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(targets))
	for _, target := range targets {
		roots = append(roots, target.Path)
	}
	return roots, nil
}

func validate(src Source) error {
	if strings.TrimSpace(src.Name) == "" {
		return domain.E(domain.KindInvalid, "存储源名称不能为空", nil)
	}
	switch src.Kind {
	case KindLocal:
		path := strings.TrimSpace(src.Config["path"])
		if path == "" {
			return domain.E(domain.KindInvalid, "本地存储源需要目录路径", nil)
		}
		info, err := os.Stat(path)
		if err != nil {
			return domain.E(domain.KindInvalid, fmt.Sprintf("目录不可访问：%s", path), nil)
		}
		if !info.IsDir() {
			return domain.E(domain.KindInvalid, fmt.Sprintf("路径不是目录：%s", path), nil)
		}
	case Kind115, KindPikPak:
		// Cloud sources are authenticated separately; nothing to validate here.
	default:
		return domain.E(domain.KindInvalid, "未知的存储源类型", nil)
	}
	return nil
}
