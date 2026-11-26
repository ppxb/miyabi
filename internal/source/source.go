package source

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/storagesource"
)

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

// Service persists the configured storage sources, one StorageSource row each.
type Service struct {
	client *ent.Client
	mounts MountProvider
}

// MountProvider lists every mounted 115 account directory.
type MountProvider interface {
	Mounts(context.Context) ([]domain.LibrarySource, error)
}

// New creates the source service; mounts resolves 115 account mount directories.
func New(client *ent.Client, mounts MountProvider) *Service {
	return &Service{client: client, mounts: mounts}
}

func (s *Service) List(ctx context.Context) ([]Source, error) {
	rows, err := s.client.StorageSource.Query().Order(ent.Asc(storagesource.FieldPosition)).All(ctx)
	if err != nil {
		return nil, err
	}
	sources := make([]Source, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, Source{ID: row.ID, Kind: Kind(row.Kind), Name: row.Name, Enabled: row.Enabled, Config: row.Config})
	}
	return sources, nil
}

func (s *Service) Replace(ctx context.Context, sources []Source) ([]Source, error) {
	if sources == nil {
		sources = []Source{}
	}
	seen := make(map[string]bool, len(sources))
	for i := range sources {
		if strings.TrimSpace(sources[i].ID) == "" {
			sources[i].ID = uuid.NewString()
		}
		if seen[sources[i].ID] {
			return nil, domain.E(domain.KindInvalid, "存储源 ID 重复", nil)
		}
		seen[sources[i].ID] = true
		if err := validate(sources[i]); err != nil {
			return nil, err
		}
	}
	// The settings page submits the whole ordered list, so replace every row.
	err := ent.WithTx(ctx, s.client, func(tx *ent.Tx) error {
		if _, err := tx.StorageSource.Delete().Exec(ctx); err != nil {
			return err
		}
		builders := make([]*ent.StorageSourceCreate, 0, len(sources))
		for i, src := range sources {
			builders = append(builders, tx.StorageSource.Create().
				SetID(src.ID).SetKind(string(src.Kind)).SetName(src.Name).
				SetEnabled(src.Enabled).SetConfig(src.Config).SetPosition(i))
		}
		return tx.StorageSource.CreateBulk(builders...).Exec(ctx)
	})
	if err != nil {
		return nil, err
	}
	return sources, nil
}

// LibrarySources returns the enabled 115 sources that have a mounted directory.
// Mounts are read once and matched by account id.
func (s *Service) LibrarySources(ctx context.Context) ([]domain.LibrarySource, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	mounted, err := s.mounts.Mounts(ctx)
	if err != nil {
		return nil, err
	}
	byAccount := make(map[string]domain.LibrarySource, len(mounted))
	for _, mount := range mounted {
		byAccount[mount.AccountID] = mount
	}
	result := make([]domain.LibrarySource, 0)
	for _, src := range sources {
		if src.Kind != Kind115 || !src.Enabled {
			continue
		}
		accountID := strings.TrimSpace(src.Config["account_id"])
		if accountID == "" {
			continue
		}
		if mount, ok := byAccount[accountID]; ok {
			result = append(result, mount)
		}
	}
	return result, nil
}

// LocalTargets returns the enabled local source directories with their mode.
func (s *Service) LocalTargets(ctx context.Context) ([]domain.LocalScanTarget, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]domain.LocalScanTarget, 0)
	for _, src := range sources {
		if src.Kind != KindLocal || !src.Enabled {
			continue
		}
		if path := strings.TrimSpace(src.Config["path"]); path != "" {
			targets = append(targets, domain.LocalScanTarget{Path: path, MetadataOnly: localMetadataOnly(src)})
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
	case Kind115:
		// Cloud sources are authenticated separately; nothing to validate here.
	case KindPikPak:
		// Reserved for a future PikPak drive; there is no login or scan yet.
		return domain.E(domain.KindInvalid, "PikPak 存储源暂不支持", nil)
	default:
		return domain.E(domain.KindInvalid, "未知的存储源类型", nil)
	}
	return nil
}
