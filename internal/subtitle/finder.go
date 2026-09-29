package subtitle

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/netx"
)

// Provider searches one online subtitle source.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]Candidate, error)
}

// Finder searches online providers and downloads their subtitles.
type Finder struct {
	providers []Provider
	client    *http.Client
}

type FinderOption func(*Finder)

// WithProviders replaces the default providers.
func WithProviders(providers ...Provider) FinderOption {
	return func(f *Finder) { f.providers = providers }
}

func NewFinder(proxyManager *netx.ProxyManager, opts ...FinderOption) *Finder {
	f := &Finder{
		providers: []Provider{
			NewXunleiProvider(proxyManager),
			NewSubtitleCatProvider(proxyManager),
		},
		client: netx.NewSafeDownloadClient(proxyManager, 20*time.Second),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Search returns the ranked candidates for a catalogue number. Release names
// may decorate the number catalogue sites use (259LUXU-1899 for LUXU-1899),
// so relaxed forms are queried only when the exact number finds nothing.
func (f *Finder) Search(ctx context.Context, code string, uncensored bool) []Candidate {
	for _, query := range codeid.Candidates(code) {
		if ranked := Rank(f.search(ctx, query), code, uncensored); len(ranked) > 0 {
			return ranked
		}
	}
	return nil
}

// search queries all providers concurrently. A failing provider must not hide
// the others' results, so its error is dropped.
func (f *Finder) search(ctx context.Context, query string) []Candidate {
	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		candidates []Candidate
		seen       = make(map[string]bool)
	)
	for _, provider := range f.providers {
		wg.Go(func() {
			results, _ := provider.Search(ctx, query)
			mu.Lock()
			defer mu.Unlock()
			for _, candidate := range results {
				if candidate.URL != "" && !seen[candidate.URL] {
					seen[candidate.URL] = true
					candidates = append(candidates, candidate)
				}
			}
		})
	}
	wg.Wait()
	return candidates
}

// Download fetches a candidate and resolves an unknown language from its text.
func (f *Finder) Download(ctx context.Context, candidate Candidate) ([]byte, subtitlemeta.Language, error) {
	if candidate.URL == "" {
		return nil, "", fmt.Errorf("empty candidate URL")
	}
	raw, err := netx.SafeDownload(ctx, f.client, candidate.URL)
	if err != nil {
		return nil, "", fmt.Errorf("download subtitle: %w", err)
	}
	body, text, err := Normalize(raw, candidate.Format)
	if err != nil {
		return nil, "", err
	}
	language := candidate.Language
	if language == subtitlemeta.LangUnknown {
		language = subtitlemeta.DetectLanguage("", text)
	}
	return body, language, nil
}
