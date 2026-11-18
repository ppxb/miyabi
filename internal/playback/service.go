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
	ID      string        `json:"id"`
	Sources []MediaSource `json:"sources"`
}

// Service manages playback sessions and stream proxying.
type Service struct {
	database *ent.Client
	drive    *drive.Drive
	mu       sync.Mutex
	sessions map[string]*playSession
	// sessionTTL releases sessions abandoned by a closed tab.
	sessionTTL time.Duration
}

func New(database *ent.Client, d *drive.Drive, sessionTTL time.Duration) *Service {
	return &Service{
		database:   database,
		drive:      d,
		sessions:   make(map[string]*playSession),
		sessionTTL: sessionTTL,
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
