package source

import (
	"context"

	"github.com/ppxb/miyabi/internal/database"
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
	client *ent.Client
}

func New(client *ent.Client) *Service {
	return &Service{client: client}
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
	if err := database.SaveSetting(ctx, s.client, settingKey, sources); err != nil {
		return nil, err
	}
	return sources, nil
}
