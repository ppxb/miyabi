// Package metadata resolves movie metadata independently of discovery and ownership.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/metadatacache"
	"golang.org/x/sync/singleflight"
)

var ErrNotFound = errors.New("metadata not found")

// Source returns confirmed identities, never the closest unverified search hit.
type Source interface {
	ID() string
	Supports(code string) bool
	Fetch(context.Context, string) (domain.MovieMetadata, error)
	Media(context.Context, string) (domain.Media, error)
}

type SourceSetting struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type Service struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closed   bool
	db       *ent.Client
	sources  map[string]Source
	mu       sync.RWMutex
	settings []SourceSetting
	requests singleflight.Group
	capacity chan struct{}
}

func New(ctx context.Context, db *ent.Client, sources ...Source) (*Service, error) {
	s := &Service{db: db, sources: make(map[string]Source), capacity: make(chan struct{}, 8)}
	s.ctx, s.cancel = context.WithCancel(ctx)
	for _, source := range sources {
		if _, exists := s.sources[source.ID()]; exists {
			return nil, fmt.Errorf("duplicate metadata source %s", source.ID())
		}
		s.sources[source.ID()] = source
		s.settings = append(s.settings, SourceSetting{ID: source.ID(), Enabled: true})
	}
	settings, found, err := database.LoadSetting[[]SourceSetting](ctx, db, "metadata.sources")
	if err != nil {
		return nil, err
	}
	if found {
		if err := s.validate(settings); err != nil {
			return nil, err
		}
		s.settings = settings
	}
	return s, nil
}

func (s *Service) Settings() []SourceSetting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.settings)
}

func (s *Service) validate(settings []SourceSetting) error {
	if len(settings) != len(s.sources) {
		return domain.E(domain.KindInvalid, "请提供完整的刮削来源列表", nil)
	}
	seen := make(map[string]bool)
	for _, item := range settings {
		if s.sources[item.ID] == nil || seen[item.ID] {
			return domain.E(domain.KindInvalid, "未知或重复的刮削来源", nil)
		}
		seen[item.ID] = true
	}
	return nil
}

func (s *Service) UpdateSettings(ctx context.Context, settings []SourceSetting) error {
	if err := s.validate(settings); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := database.SaveSetting(ctx, s.db, "metadata.sources", settings); err != nil {
		return err
	}
	s.settings = slices.Clone(settings)
	return nil
}

// Resolve preserves full-number precedence across sources. Source completion
// order cannot change the merge, and failures are never negative-cache entries.
func (s *Service) Resolve(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	settings := s.Settings()
	var failures []error
	for _, layer := range codeid.Layers(ref.Code) {
		results := make([]domain.MovieMetadata, len(settings))
		errs := make([]error, len(settings))
		var workers sync.WaitGroup
		for i, setting := range settings {
			if !setting.Enabled {
				continue
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				source := s.sources[setting.ID]
				for _, code := range layer {
					if !source.Supports(code) {
						continue
					}
					result, err := s.fetch(ctx, source, code)
					if errors.Is(err, ErrNotFound) {
						continue
					}
					if err != nil {
						errs[i] = fmt.Errorf("%s: %w", setting.ID, err)
						return
					}
					if results[i].Detail.Code != "" && !codeid.IsFormatEquivalent(result.Detail.Code, results[i].Detail.Code) {
						errs[i] = fmt.Errorf("%s: 无法唯一匹配 %s", setting.ID, ref.Code)
						results[i] = domain.MovieMetadata{}
						return
					}
					results[i] = result
				}
			}()
		}
		workers.Wait()
		if err := ctx.Err(); err != nil {
			return domain.MovieMetadata{}, err
		}
		for _, err := range errs {
			if err != nil {
				failures = append(failures, err)
			}
		}
		matchedCode := ""
		for _, result := range results {
			if result.Detail.Code == "" {
				continue
			}
			if matchedCode != "" && !codeid.IsFormatEquivalent(matchedCode, result.Detail.Code) {
				return domain.MovieMetadata{}, domain.E(domain.KindConflict, "来源匹配到了不同影片，无法自动合并", nil)
			}
			matchedCode = result.Detail.Code
		}
		merged := merge(results)
		if merged.Detail.Code != "" {
			merged.Detail.ID = ref.JavDBID
			if ref.JavDBID != "" {
				merged.Detail.Sources = append(merged.Detail.Sources, domain.SourceID{Provider: "javdb", ID: ref.JavDBID})
			}
			return merged, nil
		}
		// Network failures at a stronger layer must not silently select a weaker identity.
		if len(failures) > 0 {
			break
		}
	}
	if len(failures) > 0 {
		return domain.MovieMetadata{}, domain.E(domain.KindUpstream, "刮削来源查询失败", errors.Join(failures...))
	}
	return domain.MovieMetadata{}, domain.E(domain.KindNotFound, "已启用的来源未找到可确认的影片资料", ErrNotFound)
}

func (s *Service) fetch(ctx context.Context, source Source, code string) (domain.MovieMetadata, error) {
	key := source.ID() + ":" + codeid.Normalize(code)
	ch := s.requests.DoChan(key, func() (any, error) {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, context.Canceled
		}
		s.wg.Add(1)
		s.mu.Unlock()
		defer s.wg.Done()
		// Bound shared requests independently of one caller leaving a detail dialog.
		queryCtx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
		defer cancel()
		entry, err := s.db.MetadataCache.Query().Where(metadatacache.ProviderEQ(source.ID()), metadatacache.CodeEQ(code)).Only(queryCtx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
		if entry != nil && time.Now().Before(entry.ExpiresAt) {
			if entry.Result == nil {
				return nil, ErrNotFound
			}
			return *entry.Result, nil
		}
		select {
		case s.capacity <- struct{}{}:
		case <-queryCtx.Done():
			return nil, queryCtx.Err()
		}
		defer func() { <-s.capacity }()
		// A queued request observes source disabling before starting network I/O.
		if !s.enabled(source.ID()) {
			return nil, ErrNotFound
		}
		result, err := source.Fetch(queryCtx, code)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		ttl := 24 * time.Hour
		var cached *domain.MovieMetadata
		if err == nil {
			if !codeid.IsFormatEquivalent(result.Detail.Code, code) || result.Detail.Title == "" || len(result.Detail.Sources) == 0 {
				return nil, fmt.Errorf("来源返回了不完整或不匹配的影片身份: %s / %s", code, result.Detail.Code)
			}
			cached = &result
		} else {
			ttl = 10 * time.Minute
		}
		if saveErr := s.db.MetadataCache.Create().SetProvider(source.ID()).SetCode(code).SetResult(cached).
			SetExpiresAt(time.Now().Add(ttl)).OnConflictColumns(metadatacache.FieldProvider, metadatacache.FieldCode).UpdateNewValues().Exec(queryCtx); saveErr != nil {
			return nil, saveErr
		}
		return result, err
	})
	select {
	case <-ctx.Done():
		return domain.MovieMetadata{}, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return domain.MovieMetadata{}, result.Err
		}
		return result.Val.(domain.MovieMetadata), nil
	}
}

func (s *Service) enabled(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, setting := range s.settings {
		if setting.ID == id {
			return setting.Enabled
		}
	}
	return false
}

// Image fetches a candidate through the adapter that owns its CDN and headers.
func (s *Service) Image(ctx context.Context, candidate domain.ImageCandidate) (domain.Media, error) {
	source := s.sources[candidate.Provider]
	if source == nil {
		return domain.Media{}, domain.E(domain.KindInvalid, "未知的图片来源", nil)
	}
	select {
	case s.capacity <- struct{}{}:
	case <-ctx.Done():
		return domain.Media{}, ctx.Err()
	}
	defer func() { <-s.capacity }()
	return source.Media(ctx, candidate.URL)
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	for _, source := range s.sources {
		if closer, ok := source.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}
