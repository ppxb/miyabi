package emby

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/gfriends"
)

// MediaFetcher downloads catalogue images such as JavDB actor avatars.
type MediaFetcher interface {
	Media(context.Context, string) (domain.Media, error)
}

type actorSync struct {
	db               *ent.Client
	client           *embyClient
	config           func() Config
	gfriends         *gfriends.Client
	media            MediaFetcher
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	timer            *time.Timer
	closed           bool
	running          bool
	runCancel        context.CancelFunc
	wg               sync.WaitGroup
	avatarNotFoundMu sync.Mutex
	avatarNotFound   map[string]time.Time
}

func newActorSync(db *ent.Client, client *embyClient, config func() Config, gfriends *gfriends.Client, media MediaFetcher) *actorSync {
	ctx, cancel := context.WithCancel(context.Background())
	return &actorSync{db: db, client: client, config: config, gfriends: gfriends, media: media, ctx: ctx, cancel: cancel, avatarNotFound: make(map[string]time.Time)}
}

var errActorSyncBusy = errors.New("actor sync already running")

func (s *actorSync) schedule(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, func() {
		ctx, cancel := context.WithTimeout(s.ctx, 15*time.Minute)
		defer cancel()
		_, err := s.run(ctx)
		if errors.Is(err, errActorSyncBusy) {
			s.schedule(10 * time.Second)
		} else if err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "emby actor avatar sync finished with error", "error", err)
		}
	})
}

func (s *actorSync) run(ctx context.Context) (int, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, context.Canceled
	}
	if s.running {
		s.mu.Unlock()
		return 0, errActorSyncBusy
	}
	s.running = true
	ctx, cancel := context.WithCancel(ctx)
	s.runCancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.runCancel = nil
		s.mu.Unlock()
		s.wg.Done()
	}()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	cfg := s.config()
	if !cfg.ready() || !cfg.IsSyncActors() {
		return 0, nil
	}
	return s.syncAvatars(ctx, cfg)
}

// Configuration changes stop pending and in-flight work before scheduling a new run.
func (s *actorSync) reconfigure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.runCancel != nil {
		s.runCancel()
	}
}

func (s *actorSync) close() {
	s.mu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *actorSync) syncAvatars(ctx context.Context, cfg Config) (int, error) {
	g, media := s.gfriends, s.media
	people, err := s.client.persons(ctx, cfg)
	if err != nil {
		return 0, fmt.Errorf("list persons for avatar sync: %w", err)
	}

	if len(people) == 0 {
		return 0, nil
	}

	canCacheMiss := true
	if g != nil {
		if err := g.EnsureIndex(ctx); err != nil {
			// Skip GFriends for this run instead of re-downloading its index per actor.
			slog.WarnContext(ctx, "gfriends index unavailable; using JavDB avatars only", "error", err)
			g = nil
			canCacheMiss = false
		}
	}

	slog.InfoContext(ctx, "emby actor avatar sync started", "person_count", len(people))
	uploaded := 0

	for _, person := range people {
		if err := ctx.Err(); err != nil {
			return uploaded, err
		}

		if s.isAvatarNotFound(person.Name) {
			continue
		}
		valid, err := s.client.hasValidAvatar(ctx, cfg, person)
		if err != nil {
			if ctx.Err() != nil {
				return uploaded, ctx.Err()
			}
			// An unavailable image-details endpoint does not prove the image is broken.
			slog.WarnContext(ctx, "failed to inspect actor avatar", "name", person.Name, "error", err)
			continue
		}
		if valid {
			continue
		}

		avatar, found, lookupErr := s.findAvatar(ctx, g, media, person.Name)
		if found {
			if err := s.client.uploadAvatar(ctx, cfg, person.ID, avatar); err != nil {
				slog.WarnContext(ctx, "failed to upload avatar for actor", "name", person.Name, "error", err)
			} else {
				uploaded++
				slog.DebugContext(ctx, "uploaded actor avatar to emby", "name", person.Name)
			}
		} else if lookupErr != nil {
			slog.WarnContext(ctx, "failed to resolve actor avatar", "name", person.Name, "error", lookupErr)
		} else if canCacheMiss {
			s.markAvatarNotFound(person.Name)
		}

		// Rate limiting: sleep 200ms
		select {
		case <-ctx.Done():
			return uploaded, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	slog.InfoContext(ctx, "emby actor avatar sync completed", "uploaded", uploaded, "total_persons", len(people))
	return uploaded, nil
}

const defaultAvatarNotFoundTTL = 24 * time.Hour

func (s *actorSync) isAvatarNotFound(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	s.avatarNotFoundMu.Lock()
	defer s.avatarNotFoundMu.Unlock()
	if s.avatarNotFound == nil {
		return false
	}
	expiresAt, ok := s.avatarNotFound[name]
	if !ok {
		return false
	}
	if time.Now().After(expiresAt) {
		delete(s.avatarNotFound, name)
		return false
	}
	return true
}

func (s *actorSync) markAvatarNotFound(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.avatarNotFoundMu.Lock()
	defer s.avatarNotFoundMu.Unlock()
	if s.avatarNotFound == nil {
		s.avatarNotFound = make(map[string]time.Time)
	}
	s.avatarNotFound[name] = time.Now().Add(defaultAvatarNotFoundTTL)
}

// ClearAvatarNotFoundCache empties the negative cache for actor avatar resolution.
func (s *actorSync) clearCache() {
	s.avatarNotFoundMu.Lock()
	defer s.avatarNotFoundMu.Unlock()
	clear(s.avatarNotFound)
}

// findAvatar prefers GFriends and falls back to the JavDB avatar of a scraped actor.
func (s *actorSync) findAvatar(ctx context.Context, g *gfriends.Client, media MediaFetcher, name string) (domain.Media, bool, error) {
	if s.isAvatarNotFound(name) {
		return domain.Media{}, false, nil
	}
	var upstreamErr error
	if g != nil {
		data, err := g.FetchAvatar(ctx, name)
		if err == nil && len(data) > 0 {
			return domain.Media{ContentType: http.DetectContentType(data), Body: data}, true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			upstreamErr = err
		}
	}
	if media == nil {
		return domain.Media{}, false, upstreamErr
	}
	act, err := s.db.Actor.Query().Where(actor.Or(actor.NameEQ(name), actor.NameZhtEQ(name)), actor.AvatarNotNil()).First(ctx)
	if ent.IsNotFound(err) {
		return domain.Media{}, false, upstreamErr
	}
	if err != nil {
		return domain.Media{}, false, errors.Join(upstreamErr, err)
	}
	if *act.Avatar == "" {
		return domain.Media{}, false, upstreamErr
	}
	image, err := media.Media(ctx, *act.Avatar)
	if err != nil {
		return domain.Media{}, false, errors.Join(upstreamErr, err)
	}
	return image, true, nil
}
