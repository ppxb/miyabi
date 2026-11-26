package drive

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

const (
	mediaCacheLimit     = 256
	mediaCacheTTL       = time.Minute
	mediaExpiryMargin   = 30 * time.Second
	mediaResolveTimeout = 45 * time.Second
)

// MediaSession is the session subset required to resolve playable media.
type MediaSession interface {
	AuthorizationVersion() uint64
	Info(ctx context.Context, fileID string) (pan.FileInfo, error)
	DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error)
	PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error)
	WithSource(ctx context.Context, fn func() error) error
}

// Media is a resolved 115 file: a direct download URL or transcoded streams.
type Media struct {
	DownloadURL string
	Streams     []pan.PlaySource
}

type mediaCacheEntry struct {
	media   Media
	expires time.Time
}

// mediaResolver shares recent direct URLs between web playback and the STRM relay.
type mediaResolver struct {
	group singleflight.Group
	mu    sync.Mutex
	urls  map[string]mediaCacheEntry
}

// ResolveMedia prefers a direct download URL and falls back to transcoded
// streams. Concurrent callers for the same source, file and user agent share one
// lookup; results are cached only until their signature expires.
func (d *Drive) ResolveMedia(ctx context.Context, sess MediaSession, accountID, fileID, pickCode, userAgent string) (Media, error) {
	ua := strings.TrimSpace(userAgent)
	key := fmt.Sprintf("%d:%q:%q:%q", sess.AuthorizationVersion(), accountID, fileID, ua)
	result := d.media.group.DoChan(key, func() (any, error) {
		done, ok := d.StartWork()
		if !ok {
			return Media{}, context.Canceled
		}
		defer done()
		// One caller's canceled request must not cancel another caller's lookup.
		resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaResolveTimeout)
		defer cancel()
		if media, ok := d.media.cached(key, time.Now()); ok {
			return media, nil
		}
		media, err := resolveMedia(resolveCtx, sess, fileID, pickCode, ua)
		if err != nil {
			return Media{}, err
		}
		d.media.remember(key, media, time.Now())
		return media, nil
	})
	select {
	case <-ctx.Done():
		return Media{}, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return Media{}, completed.Err
		}
		media := completed.Val.(Media)
		// Re-check after a cache hit or a lookup that outlived a mount change.
		if err := sess.WithSource(ctx, func() error { return nil }); err != nil {
			return Media{}, err
		}
		return media, nil
	}
}

func resolveMedia(ctx context.Context, sess MediaSession, fileID, pickCode, userAgent string) (Media, error) {
	if pickCode == "" {
		info, err := sess.Info(ctx, fileID)
		if err != nil {
			return Media{}, fmt.Errorf("read 115 video info: %w", err)
		}
		if info.PickCode == "" {
			return Media{}, domain.E(domain.KindNotFound, "未能获取到视频的 115 提取码", nil)
		}
		pickCode = info.PickCode
	}
	// Prefer the direct download URL for full CDN throughput and instant seeking.
	address, err := sess.DownloadURL(ctx, pickCode, userAgent)
	if err == nil {
		return Media{DownloadURL: address}, nil
	}
	if !errors.Is(err, pan.ErrDownloadUnavailable) {
		return Media{}, fmt.Errorf("get 115 download URL: %w", err)
	}
	streams, err := sess.PlayURL(ctx, pickCode, userAgent)
	if err != nil {
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return Media{}, ErrTranscodeUnavailable
		}
		return Media{}, fmt.Errorf("get 115 playback URL: %w", err)
	}
	return Media{Streams: streams}, nil
}

func (resolver *mediaResolver) cached(key string, now time.Time) (Media, bool) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	for key, entry := range resolver.urls {
		if !now.Before(entry.expires) {
			delete(resolver.urls, key)
		}
	}
	entry, ok := resolver.urls[key]
	return entry.media, ok
}

func (resolver *mediaResolver) remember(key string, media Media, now time.Time) {
	expires := mediaExpiry(media, now)
	if !now.Before(expires) {
		return
	}
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.urls == nil {
		resolver.urls = make(map[string]mediaCacheEntry)
	}
	for key, entry := range resolver.urls {
		if !now.Before(entry.expires) {
			delete(resolver.urls, key)
		}
	}
	if len(resolver.urls) >= mediaCacheLimit {
		var oldest string
		var deadline time.Time
		for key, entry := range resolver.urls {
			if deadline.IsZero() || entry.expires.Before(deadline) {
				oldest, deadline = key, entry.expires
			}
		}
		delete(resolver.urls, oldest)
	}
	resolver.urls[key] = mediaCacheEntry{media: media, expires: expires}
}

// mediaExpiry returns the earliest signature deadline. Any stream with an
// unknown expiry makes the whole result unsafe to cache.
func mediaExpiry(media Media, now time.Time) time.Time {
	if media.DownloadURL != "" {
		return mediaCacheExpiry(media.DownloadURL, now)
	}
	var expires time.Time
	for _, stream := range media.Streams {
		deadline := mediaCacheExpiry(stream.URL, now)
		if deadline.IsZero() {
			return time.Time{}
		}
		if expires.IsZero() || deadline.Before(expires) {
			expires = deadline
		}
	}
	return expires
}

// 115 download links carry a Unix expiry in t; explicit expires timestamps are
// also accepted. Unknown or malformed expiry formats are never persisted in the
// cache. Keep the original URL intact and stop reusing it before its signature ends.
func mediaCacheExpiry(address string, now time.Time) time.Time {
	parsed, err := url.Parse(address)
	if err != nil {
		return time.Time{}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return time.Time{}
	}
	expires := now.Add(mediaCacheTTL)
	known := false
	for _, name := range []string{"t", "expires"} {
		for _, value := range query[name] {
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil || seconds <= 0 {
				return time.Time{}
			}
			deadline := time.Unix(seconds, 0).Add(-mediaExpiryMargin)
			if deadline.Before(expires) {
				expires = deadline
			}
			known = true
		}
	}
	if !known {
		return time.Time{}
	}
	return expires
}
