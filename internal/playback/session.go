package playback

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/ppxb/miyabi/internal/domain"
)

type playResource struct {
	url      *url.URL
	playlist bool
}

type mediaSpec struct {
	address   string
	mediaType string
	label     string
	playlist  bool
}

type playSession struct {
	id        string
	source    domain.LibrarySource
	ctx       context.Context
	cancel    context.CancelFunc
	timer     *time.Timer
	resources []playResource
	byURL     map[string]int
	// proxySegments relays HLS segment bytes through this server (same-origin) so
	// browser MSE isn't blocked by the 115 CDN's fixed CORS origin.
	proxySegments bool
}

func (service *Service) createSession(source domain.LibrarySource, specs []mediaSpec) (Playback, error) {
	session := &playSession{id: uuid.NewString(), source: source, byURL: make(map[string]int)}
	for _, spec := range specs {
		if spec.playlist {
			session.proxySegments = true
		}
	}
	result := Playback{ID: session.id, Source: source, Sources: make([]MediaSource, 0, len(specs))}
	for _, spec := range specs {
		address, err := url.Parse(spec.address)
		if err != nil {
			return Playback{}, fmt.Errorf("115 returned an invalid playback URL")
		}
		local, err := session.register(address, spec.playlist)
		if err != nil {
			return Playback{}, err
		}
		result.Sources = append(result.Sources, MediaSource{Src: local, Type: spec.mediaType, Label: spec.label})
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	session.ctx, session.cancel = context.WithCancel(context.Background())
	service.sessions[session.id] = session
	// Explicit close releases immediately; the timer clears sessions abandoned by a closed tab.
	session.timer = time.AfterFunc(service.sessionTTL, func() { service.Release(session.id) })
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

func (service *Service) Release(id string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if session, exists := service.sessions[id]; exists {
		session.timer.Stop()
		session.cancel()
		delete(service.sessions, id)
	}
}

func (service *Service) resource(ctx context.Context, id string, index int) (*playSession, playResource, error) {
	service.mu.Lock()
	session, exists := service.sessions[id]
	if !exists || index < 0 || index >= len(session.resources) {
		service.mu.Unlock()
		return nil, playResource{}, domain.E(domain.KindNotFound, "播放会话已结束，请重新播放", fs.ErrNotExist)
	}
	resource := session.resources[index]
	service.mu.Unlock()

	if _, err := service.drive.OpenSource(ctx, session.source); err != nil {
		service.Release(id)
		return nil, playResource{}, domain.E(domain.KindNotFound, "登录账号或媒体目录已变更，请重新播放", fs.ErrNotExist)
	}
	return session, resource, nil
}
