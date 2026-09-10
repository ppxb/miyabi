package service

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/pan"
)

type PlayFiles struct {
	Code  string        `json:"code"`
	Title string        `json:"title"`
	Files []LibraryFile `json:"files"`
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

type playResource struct {
	url      *url.URL
	playlist bool
}

type playSession struct {
	id        string
	source    LibrarySource
	version   uint64
	ctx       context.Context
	cancel    context.CancelFunc
	timer     *time.Timer
	resources []playResource
	byURL     map[string]int
}

type PlayService struct {
	library  *LibraryService
	mu       sync.Mutex
	sessions map[string]*playSession
}

func NewPlayService(library *LibraryService) *PlayService {
	return &PlayService{library: library, sessions: make(map[string]*playSession)}
}

func (service *PlayService) Files(ctx context.Context, movieID int) (PlayFiles, error) {
	source, err := loadLibrarySource(ctx, service.library.database)
	if err != nil {
		return PlayFiles{}, err
	}
	if source == nil {
		return PlayFiles{}, ErrMediaDirectoryRequired
	}
	scope := libraryFiles(*source)
	record, err := service.library.database.Movie.Query().
		Where(movie.IDEQ(movieID), movie.HasFilesWith(scope)).
		WithFiles(func(query *ent.FileQuery) {
			query.Where(scope).Order(ent.Desc(file.FieldSize), ent.Asc(file.FieldPath), ent.Asc(file.FieldID))
		}).Only(ctx)
	if err != nil {
		return PlayFiles{}, fmt.Errorf("read playable movie: %w", err)
	}
	result := PlayFiles{Code: record.Code, Title: record.Title, Files: make([]LibraryFile, 0, len(record.Edges.Files))}
	for _, entry := range record.Edges.Files {
		result.Files = append(result.Files, LibraryFile{ID: entry.FileID, Name: entry.Name, Path: entry.Path, Size: entry.Size})
	}
	return result, nil
}

func (service *PlayService) Start(ctx context.Context, fileID string) (Playback, error) {
	drive := service.library.drive
	state, err := drive.verifiedSource(ctx)
	if err != nil {
		return Playback{}, err
	}
	source := state.source()
	_, err = service.library.database.File.Query().Where(libraryFiles(source), file.FileIDEQ(fileID)).Only(ctx)
	if err != nil {
		return Playback{}, fmt.Errorf("read indexed video: %w", err)
	}
	info, err := withPanSourceToken(ctx, drive, state, func(token string) (pan.FileInfo, error) {
		return drive.client.Info(ctx, token, fileID)
	})
	if err != nil {
		return Playback{}, fmt.Errorf("read 115 video: %w", err)
	}
	if info.IsDirectory || !isVideo(info.Name) || !withinSource(info, source) {
		return Playback{}, fmt.Errorf("视频已不在当前媒体目录中，请重新扫描: %w", fs.ErrNotExist)
	}
	if info.PickCode == "" {
		return Playback{}, fmt.Errorf("115 returned no pick code for video")
	}
	if err := service.library.checkScanSource(source, state.authorizationVersion); err != nil {
		return Playback{}, err
	}
	sources, err := withPanSourceToken(ctx, drive, state, func(token string) ([]pan.PlaySource, error) {
		return drive.client.PlayURL(ctx, token, info.PickCode)
	})
	if err != nil {
		return Playback{}, fmt.Errorf("get 115 playback URL: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Playback{}, err
	}
	if err := service.library.checkScanSource(source, state.authorizationVersion); err != nil {
		return Playback{}, err
	}
	return service.createSession(source, state.authorizationVersion, sources)
}

func (service *PlayService) createSession(source LibrarySource, version uint64, sources []pan.PlaySource) (Playback, error) {
	session := &playSession{id: uuid.NewString(), source: source, version: version, byURL: make(map[string]int)}
	result := Playback{ID: session.id, Sources: make([]MediaSource, 0, len(sources))}
	for _, source := range sources {
		address, err := url.Parse(source.URL)
		if err != nil {
			return Playback{}, fmt.Errorf("115 returned an invalid playback URL")
		}
		local, err := session.register(address, true)
		if err != nil {
			return Playback{}, err
		}
		item := MediaSource{Src: local, Type: "application/x-mpegurl", Label: fmt.Sprintf("%dp", source.Height)}
		if source.Definition == 100 {
			item.Label = "原画 · " + item.Label
		}
		result.Sources = append(result.Sources, item)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	session.ctx, session.cancel = context.WithCancel(context.Background())
	service.sessions[session.id] = session
	// Explicit close releases immediately; this also clears sessions abandoned by a closed tab.
	session.timer = time.AfterFunc(8*time.Hour, func() { service.Release(session.id) })
	return result, nil
}

// Only URLs returned by 115 or referenced by its playlists become proxy resources.
// The browser receives opaque local paths, never signed upstream URLs.
func (session *playSession) register(address *url.URL, playlist bool) (string, error) {
	if (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" || address.User != nil {
		return "", fmt.Errorf("115 returned an unsupported media URL")
	}
	key := address.String()
	index, exists := session.byURL[key]
	if !exists {
		index = len(session.resources)
		session.byURL[key] = index
		session.resources = append(session.resources, playResource{url: address, playlist: playlist})
	} else if playlist {
		session.resources[index].playlist = true
	}
	return "/api/play/" + session.id + "/stream/" + strconv.Itoa(index), nil
}

func (service *PlayService) Release(id string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if session, exists := service.sessions[id]; exists {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

func (service *PlayService) Close() {
	service.mu.Lock()
	defer service.mu.Unlock()
	for id, session := range service.sessions {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

func (service *PlayService) resource(id string, index int) (*playSession, playResource, error) {
	service.mu.Lock()
	session, exists := service.sessions[id]
	if !exists || index < 0 || index >= len(session.resources) {
		service.mu.Unlock()
		return nil, playResource{}, fmt.Errorf("播放会话已结束，请重新播放: %w", fs.ErrNotExist)
	}
	resource := session.resources[index]
	service.mu.Unlock()

	state := service.library.drive.snapshot()
	valid := !state.closed && state.matchesSource(session.source, session.version) && state.tokens.AccessToken != ""
	if !valid {
		service.Release(id)
		return nil, playResource{}, fmt.Errorf("登录账号或媒体目录已变更，请重新播放: %w", fs.ErrNotExist)
	}
	return session, resource, nil
}
