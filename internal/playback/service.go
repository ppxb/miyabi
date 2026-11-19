package playback

import (
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
)

type PlayFiles struct {
	Code   string               `json:"code"`
	Title  string               `json:"title"`
	Files  []domain.LibraryFile `json:"files"`
	Source domain.LibrarySource `json:"source"`
}

type MediaSource struct {
	Src   string `json:"src"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type Playback struct {
	ID      string               `json:"id"`
	FileID  string               `json:"file_id"`
	Sources []MediaSource        `json:"sources"`
	Source  domain.LibrarySource `json:"source"`
	Resume  *domain.WatchResume  `json:"resume,omitempty"`
}

// Service manages playback sessions and stream proxying.
type Service struct {
	database *ent.Client
	drive    *drive.Drive
	mu       sync.Mutex
	sessions map[string]*playSession
	// sessionTTL releases sessions abandoned by a closed tab.
	sessionTTL time.Duration
	// urlCache briefly reuses resolved 115 direct URLs so reopening the same
	// file starts without another round trip. Guarded by urlMu.
	urlMu    sync.Mutex
	urlCache map[string]urlCacheEntry
}

// downloadURLCacheTTL keeps resolved direct URLs only briefly; 115 signed URLs
// eventually expire and must be re-resolved rather than served stale.
const downloadURLCacheTTL = 2 * time.Minute

type urlCacheEntry struct {
	address string
	expires time.Time
}

func New(database *ent.Client, d *drive.Drive, sessionTTL time.Duration) *Service {
	return &Service{
		database:   database,
		drive:      d,
		sessions:   make(map[string]*playSession),
		sessionTTL: sessionTTL,
		urlCache:   make(map[string]urlCacheEntry),
	}
}

func (service *Service) Close() {
	service.mu.Lock()
	defer service.mu.Unlock()
	for id, session := range service.sessions {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

func (service *Service) cachedDownloadURL(key string) (string, bool) {
	service.urlMu.Lock()
	defer service.urlMu.Unlock()
	entry, ok := service.urlCache[key]
	if !ok || time.Now().After(entry.expires) {
		return "", false
	}
	return entry.address, true
}

func (service *Service) storeDownloadURL(key, address string) {
	service.urlMu.Lock()
	defer service.urlMu.Unlock()
	if service.urlCache == nil {
		service.urlCache = make(map[string]urlCacheEntry)
	}
	service.urlCache[key] = urlCacheEntry{address: address, expires: time.Now().Add(downloadURLCacheTTL)}
}
