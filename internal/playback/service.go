package playback

import (
	"context"
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
	// localRoots lists the configured local source directories, used to turn an
	// indexed relative path back into an absolute path for local playback.
	localRoots func(context.Context) ([]string, error)
}

func New(database *ent.Client, d *drive.Drive, sessionTTL time.Duration, localRoots func(context.Context) ([]string, error)) *Service {
	return &Service{
		database:   database,
		drive:      d,
		sessions:   make(map[string]*playSession),
		sessionTTL: sessionTTL,
		localRoots: localRoots,
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
