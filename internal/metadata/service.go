// Package metadata resolves movie metadata independently of discovery and ownership.
package metadata

import (
	"cmp"
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
	Fetch(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
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
		if source.ID() != "javdb" {
			s.settings = append(s.settings, SourceSetting{ID: source.ID(), Enabled: true})
		}
	}
	settings, found, err := database.LoadSetting[[]SourceSetting](ctx, db, "metadata.sources")
	if err != nil {
		return nil, err
	}
	if found {
		// The registry defines available sources. Preserve their saved order and
		// switches, discard removed sources, then append newly registered ones.
		registered := make(map[string]bool, len(s.sources))
		var current []SourceSetting
		for _, setting := range settings {
			if setting.ID != "javdb" && s.sources[setting.ID] != nil {
				current = append(current, setting)
				registered[setting.ID] = true
			}
		}
		for _, setting := range s.settings {
			if !registered[setting.ID] {
				current = append(current, setting)
			}
		}
		s.settings = current
	}
	slices.SortStableFunc(s.settings, func(a, b SourceSetting) int {
		return cmp.Compare(sourceOrder(a.ID), sourceOrder(b.ID))
	})
	if err := s.validate(s.settings); err != nil {
		return nil, err
	}
	return s, nil
}

func sourceOrder(id string) int {
	switch id {
	case "fanza":
		return -1
	default:
		return 0
	}
}

func (s *Service) Settings() []SourceSetting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.settings)
}

func (s *Service) validate(settings []SourceSetting) error {
	count := len(s.sources)
	if s.sources["javdb"] != nil {
		count--
	}
	if len(settings) != count {
		return domain.E(domain.KindInvalid, "请提供完整的刮削来源列表", nil)
	}
	seen := make(map[string]bool)
	for i, item := range settings {
		if item.ID == "javdb" || s.sources[item.ID] == nil || seen[item.ID] {
			return domain.E(domain.KindInvalid, "未知或重复的刮削来源", nil)
		}
		seen[item.ID] = true
		if i > 0 && sourceOrder(settings[i-1].ID) > sourceOrder(item.ID) {
			return domain.E(domain.KindInvalid, "FANZA 必须为首选来源", nil)
		}
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

// Resolve checks each identity layer across sources. JavDB is always queried
// for catalogue identities; official sources supply artwork and missing fields.
// A network failure never permits a weaker identity.
func (s *Service) Resolve(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	settings := s.Settings()
	if s.sources["javdb"] != nil {
		settings = append(settings, SourceSetting{ID: "javdb", Enabled: true})
	}
	var failures []error
	for _, layer := range codeid.Layers(ref.Code) {
		var results []domain.MovieMetadata
		var merged domain.MovieMetadata
		for _, setting := range settings {
			if !setting.Enabled || (setting.ID != "javdb" && merged.Detail.Code != "" && !needsSupplement(merged)) {
				continue
			}
			result, err := s.resolveLayer(ctx, s.sources[setting.ID], ref, layer)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					failures = append(failures, fmt.Errorf("%s: %w", setting.ID, err))
				}
				continue
			}
			if merged.Detail.Code != "" && !codeid.IsFormatEquivalent(merged.Detail.Code, result.Detail.Code) {
				return domain.MovieMetadata{}, domain.E(domain.KindConflict, "来源匹配到了不同影片，无法自动合并", nil)
			}
			results = append(results, result)
			merged = merge(results)
		}
		if err := ctx.Err(); err != nil {
			return domain.MovieMetadata{}, err
		}
		if merged.Detail.Code != "" {
			return attachJavDBIdentity(merged, ref.JavDBID)
		}
		if len(failures) > 0 {
			break
		}
	}
	if len(failures) > 0 {
		return domain.MovieMetadata{}, domain.E(domain.KindUpstream, "刮削来源查询失败", errors.Join(failures...))
	}
	return domain.MovieMetadata{}, domain.E(domain.KindNotFound, "已启用的来源未找到可确认的影片资料", ErrNotFound)
}

func (s *Service) resolveLayer(ctx context.Context, source Source, ref domain.MovieRef, codes []string) (domain.MovieMetadata, error) {
	var matched domain.MovieMetadata
	for _, code := range codes {
		if !source.Supports(code) {
			continue
		}
		ref.Code = code
		result, err := s.fetch(ctx, source, ref)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return domain.MovieMetadata{}, err
		}
		if matched.Detail.Code != "" && !codeid.IsFormatEquivalent(matched.Detail.Code, result.Detail.Code) {
			return domain.MovieMetadata{}, domain.E(domain.KindConflict, "同层候选无法唯一匹配影片", nil)
		}
		matched = result
	}
	if matched.Detail.Code == "" {
		return matched, ErrNotFound
	}
	return matched, nil
}

// Optional series, director and rating may legitimately be absent.
// They do not force another supplementary source request.
func needsSupplement(result domain.MovieMetadata) bool {
	m := result.Detail
	cover, preview := false, false
	for _, candidate := range result.Images {
		cover = cover || candidate.Role == "cover" || candidate.Role == "poster"
		preview = preview || candidate.Role == "preview"
	}
	return m.Title == "" || m.ReleaseDate == "" || m.Duration == 0 ||
		len(m.Actors) == 0 || m.Maker == nil || len(m.Tags) == 0 || !cover || !preview
}

func attachJavDBIdentity(result domain.MovieMetadata, knownID string) (domain.MovieMetadata, error) {
	for _, source := range result.Detail.Sources {
		if source.Provider == "javdb" {
			if knownID != "" && knownID != source.ID {
				return domain.MovieMetadata{}, domain.E(domain.KindConflict, "JavDB 身份与已有记录不一致", nil)
			}
			result.Detail.ID = source.ID
			return result, nil
		}
	}
	result.Detail.ID = knownID
	if knownID != "" {
		result.Detail.Sources = append(result.Detail.Sources, domain.SourceID{Provider: "javdb", ID: knownID})
	}
	return result, nil
}

// Fallback is used after independent artwork downloads fail. It checks only
// the confirmed code, never downgrading to another film to obtain an image.
func (s *Service) Fallback(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	source := s.sources["javdb"]
	if source == nil {
		return domain.MovieMetadata{}, ErrNotFound
	}
	ref.Code = codeid.Normalize(ref.Code)
	result, err := s.fetch(ctx, source, ref)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return attachJavDBIdentity(result, ref.JavDBID)
}

func (s *Service) fetch(ctx context.Context, source Source, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	ref.Code = code
	key := source.ID() + ":" + code
	knownJavDB := source.ID() == "javdb" && ref.JavDBID != ""
	if knownJavDB {
		key += ":" + ref.JavDBID
	}
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
		cacheMatches := entry != nil && time.Now().Before(entry.ExpiresAt)
		// A known ID can retrieve a film absent from search. A cached miss or a
		// different catalogue ID must not suppress that more precise lookup.
		if cacheMatches && knownJavDB {
			cacheMatches = entry.Result != nil && entry.Result.Detail.ID == ref.JavDBID
		}
		if cacheMatches {
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
		result, err := source.Fetch(queryCtx, ref)
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
	if id == "javdb" {
		return s.sources[id] != nil
	}
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
