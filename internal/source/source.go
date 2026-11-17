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
	onLocalScan func(context.Context, string) error
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
}

// SetLocalScanHandler lets the composition root enqueue scans after sources change.
func (s *Service) SetLocalScanHandler(handler func(context.Context, string) error) {
	s.onLocalScan = handler
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
	s.triggerLocalScans(ctx, sources)
	return sources, nil
}

func (s *Service) triggerLocalScans(ctx context.Context, sources []Source) {
	if s.onLocalScan == nil {
		return
	}
	for _, src := range sources {
		if src.Kind != KindLocal || !src.Enabled {
			continue
		}
		if path := strings.TrimSpace(src.Config["path"]); path != "" {
			_ = s.onLocalScan(ctx, path)
		}
	}
}

// LocalRoots returns the enabled local source directories, for startup scanning.
func (s *Service) LocalRoots(ctx context.Context) ([]string, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0)
	for _, src := range sources {
		if src.Kind != KindLocal || !src.Enabled {
			continue
		}
		if path := strings.TrimSpace(src.Config["path"]); path != "" {
			roots = append(roots, path)
		}
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
