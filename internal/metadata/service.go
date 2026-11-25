// Package metadata resolves movie metadata independently of discovery and ownership.
package metadata

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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

type Service struct {
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	closed   bool
	db       *ent.Client
	sources  map[string]Source
	mu       sync.RWMutex
	settings []domain.SourceToggle
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
		s.settings = append(s.settings, domain.SourceToggle{ID: source.ID(), Enabled: true})
	}
	settings, found, err := database.LoadSetting[[]domain.SourceToggle](ctx, db, "metadata.sources")
	if err != nil {
		return nil, err
	}
	if found {
		s.settings = domain.MergeSourceToggles(settings, s.settings)
	}
	slices.SortStableFunc(s.settings, func(a, b domain.SourceToggle) int {
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
		return -2
	case "javdb":
		return -1
	case "freejavbt":
		return 1
	default:
		return 0
	}
}

func (s *Service) Settings() []domain.SourceToggle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.settings)
}

func (s *Service) validate(settings []domain.SourceToggle) error {
	if err := domain.ValidateSourceToggles(settings, s.sources, "刮削来源"); err != nil {
		return err
	}
	for i, item := range settings {
		if i > 0 && sourceOrder(settings[i-1].ID) > sourceOrder(item.ID) {
			return domain.E(domain.KindInvalid, "FANZA 必须为首选来源", nil)
		}
	}
	return nil
}

func (s *Service) UpdateSettings(ctx context.Context, settings []domain.SourceToggle) error {
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

// Resolve queries every enabled source for each identity layer concurrently and
// merges the collected results in source order. No source is privileged; a
// network failure never permits a weaker identity.
func (s *Service) Resolve(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	return s.resolve(ctx, ref, nil)
}

// ResolveExcept behaves like Resolve but skips the given source IDs, letting a
// caller resolve detail without a particular provider (for example JavDB).
func (s *Service) ResolveExcept(ctx context.Context, ref domain.MovieRef, excluded ...string) (domain.MovieMetadata, error) {
	var skip map[string]bool
	if len(excluded) > 0 {
		skip = make(map[string]bool, len(excluded))
		for _, id := range excluded {
			skip[id] = true
		}
	}
	return s.resolve(ctx, ref, skip)
}

func (s *Service) resolve(ctx context.Context, ref domain.MovieRef, skip map[string]bool) (domain.MovieMetadata, error) {
	settings := s.Settings()
	var failures []error
	var failedSources []string
	answered := false
	for _, layer := range codeid.Layers(ref.Code) {
		// Query every enabled source for this layer concurrently, then merge the
		// collected results in source order. Waiting for all sources keeps latency
		// near the slowest single source instead of summing them; fetch already
		// bounds concurrency via singleflight + a capacity semaphore.
		type outcome struct {
			id     string
			result domain.MovieMetadata
			err    error
			used   bool
		}
		outcomes := make([]outcome, len(settings))
		var wg sync.WaitGroup
		for i, setting := range settings {
			if !setting.Enabled || skip[setting.ID] {
				continue
			}
			source := s.sources[setting.ID]
			if source == nil {
				continue
			}
			outcomes[i] = outcome{id: setting.ID, used: true}
			wg.Add(1)
			go func(index int, source Source) {
				defer wg.Done()
				result, err := s.resolveLayer(ctx, source, ref, layer)
				outcomes[index].result = result
				outcomes[index].err = err
			}(i, source)
		}
		wg.Wait()
		var results []domain.MovieMetadata
		var confirmedCode string
		for _, item := range outcomes {
			if !item.used {
				continue
			}
			if item.err != nil {
				if errors.Is(item.err, ErrNotFound) {
					answered = true
				} else {
					failures = append(failures, fmt.Errorf("%s: %w", item.id, item.err))
					if !slices.Contains(failedSources, item.id) {
						failedSources = append(failedSources, item.id)
					}
				}
				continue
			}
			if code := item.result.Detail.Code; code != "" {
				if confirmedCode == "" {
					confirmedCode = code
				} else if !codeid.IsFormatEquivalent(confirmedCode, code) {
					return domain.MovieMetadata{}, domain.E(domain.KindConflict, "来源匹配到了不同影片，无法自动合并", nil)
				}
			}
			results = append(results, item.result)
		}
		var merged domain.MovieMetadata
		if len(results) > 0 {
			merged = merge(results)
		}
		if err := ctx.Err(); err != nil {
			return domain.MovieMetadata{}, err
		}
		if merged.Detail.Code != "" {
			return attachSourceIdentity(merged), nil
		}
		if len(failures) > 0 {
			break
		}
	}
	if len(failures) > 0 {
		// A javdb outage must not fail movies that the other enabled sources
		// simply do not carry; they are "not found", not a network outage.
		if answered && onlyJavDB(failedSources) {
			return domain.MovieMetadata{}, domain.E(domain.KindNotFound, "已启用的来源未找到可确认的影片资料", ErrNotFound)
		}
		message := "刮削来源查询失败: " + strings.Join(failedSources, "、") + "，请检查网络和代理设置"
		return domain.MovieMetadata{}, domain.E(domain.KindUpstream, message, errors.Join(failures...))
	}
	return domain.MovieMetadata{}, domain.E(domain.KindNotFound, "已启用的来源未找到可确认的影片资料", ErrNotFound)
}

func onlyJavDB(sources []string) bool {
	return len(sources) == 1 && sources[0] == "javdb"
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

// attachSourceIdentity surfaces the JavDB catalogue id when a JavDB source is
// present (some callers rely on it); otherwise the movie id stays empty. JavDB
// is optional: when absent, nothing breaks.
func attachSourceIdentity(result domain.MovieMetadata) domain.MovieMetadata {
	result.Detail.ID = ""
	for _, source := range result.Detail.Sources {
		if source.Provider == "javdb" {
			result.Detail.ID = source.ID
			break
		}
	}
	return result
}

// Fallback is used after independent artwork downloads fail. It checks only
// the confirmed code, never downgrading to another film to obtain an image.
func (s *Service) Fallback(ctx context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	if !s.enabled("javdb") {
		return domain.MovieMetadata{}, ErrNotFound
	}
	source := s.sources["javdb"]
	if source == nil {
		return domain.MovieMetadata{}, ErrNotFound
	}
	ref.Code = codeid.Normalize(ref.Code)
	result, err := s.fetch(ctx, source, ref)
	if err != nil {
		return domain.MovieMetadata{}, err
	}
	return attachSourceIdentity(result), nil
}

func (s *Service) fetch(ctx context.Context, source Source, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := codeid.Normalize(ref.Code)
	ref.Code = code
	key := source.ID() + ":" + code
	if ref.Refresh {
		key += ":refresh"
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
		// JavDB is prone to risk-control stalls, so give it a shorter timeout: with
		// concurrent sources a slow JavDB no longer blocks the others.
		timeout := 25 * time.Second
		if source.ID() == "javdb" {
			timeout = 8 * time.Second
		}
		queryCtx, cancel := context.WithTimeout(s.ctx, timeout)
		defer cancel()
		entry, err := s.db.MetadataCache.Query().Where(metadatacache.ProviderEQ(source.ID()), metadatacache.CodeEQ(code)).Only(queryCtx)
		if err != nil && !ent.IsNotFound(err) {
			return nil, err
		}
		cacheMatches := !ref.Refresh && entry != nil && time.Now().Before(entry.ExpiresAt)
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
		// Successful source results are reused by later scrapes (e.g. after a
		// movie is added to the drive), so keep them for a week; negative results
		// expire quickly so a temporarily missing title is retried soon.
		ttl := 7 * 24 * time.Hour
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
