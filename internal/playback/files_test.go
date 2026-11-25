package playback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type stubSourceSession struct {
	download    func(context.Context, string, string) (string, error)
	play        func(context.Context, string, string) ([]pan.PlaySource, error)
	authVersion uint64
}

func (s stubSourceSession) AuthorizationVersion() uint64 { return s.authVersion }

func (s stubSourceSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return s.download(ctx, pickCode, userAgent)
}

func (s stubSourceSession) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return s.play(ctx, pickCode, userAgent)
}

func TestMediaSpecsPrefersDirectDownload(t *testing.T) {
	specs, err := mediaSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "https://cdn.example/video.mp4", nil },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			t.Fatal("playback URL must not be requested when a download URL exists")
			return nil, nil
		},
	}, "pick", "ua")
	if err != nil || len(specs) != 1 {
		t.Fatalf("specs=%+v err=%v", specs, err)
	}
	if specs[0].mediaType != "video/mp4" || specs[0].address != "https://cdn.example/video.mp4" || specs[0].playlist || specs[0].label != "原画" {
		t.Fatalf("unexpected direct spec: %+v", specs[0])
	}
}

func TestMediaSpecsFallsBackToHLS(t *testing.T) {
	specs, err := mediaSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return []pan.PlaySource{
				{URL: "https://cdn.example/1080.m3u8", Height: 1080, Definition: 1},
				{URL: "https://cdn.example/2160.m3u8", Height: 2160, Definition: 100},
			}, nil
		},
	}, "pick", "ua")
	if err != nil || len(specs) != 2 {
		t.Fatalf("specs=%+v err=%v", specs, err)
	}
	if specs[0].mediaType != "application/x-mpegurl" || !specs[0].playlist || specs[0].label != "1080p" {
		t.Fatalf("unexpected first spec: %+v", specs[0])
	}
	if specs[1].label != "原画 · 2160p" || !specs[1].playlist {
		t.Fatalf("unexpected original spec: %+v", specs[1])
	}
}

func TestMediaSpecsReportsDownloadErrors(t *testing.T) {
	cause := errors.New("network down")
	if _, err := mediaSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", cause },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, nil
		},
	}, "pick", "ua"); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped download failure", err)
	}
}

func TestMediaSpecsReportsTranscodeUnavailable(t *testing.T) {
	_, err := mediaSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, pan.ErrTranscodeUnavailable
		},
	}, "pick", "ua")
	if !errors.Is(err, drive.ErrTranscodeUnavailable) {
		t.Fatalf("error = %v, want transcode unavailable", err)
	}
}

func TestResolveSpecsCachesDirectURL(t *testing.T) {
	downloads := 0
	sess := stubSourceSession{
		download: func(context.Context, string, string) (string, error) {
			downloads++
			return "https://cdn.example/video.mp4", nil
		},
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			t.Fatal("playback URL must not be requested when a download URL exists")
			return nil, nil
		},
	}
	service := New(nil, nil, time.Minute, nil)
	for i := 0; i < 3; i++ {
		specs, err := service.resolveSpecs(context.Background(), sess, "acct", "pick", "ua")
		if err != nil || len(specs) != 1 || specs[0].address != "https://cdn.example/video.mp4" {
			t.Fatalf("iteration %d: specs=%+v err=%v", i, specs, err)
		}
	}
	if downloads != 1 {
		t.Fatalf("DownloadURL called %d times, want 1 (second and third served from cache)", downloads)
	}
}

// 115 direct URLs are bound to the requesting user agent and authorization, so
// another browser or a new login must resolve its own URL.
func TestResolveSpecsCacheIsScopedToUserAgentAndAuthorization(t *testing.T) {
	downloads := 0
	download := func(_ context.Context, _ string, userAgent string) (string, error) {
		downloads++
		return "https://cdn.example/video.mp4?ua=" + userAgent, nil
	}
	play := func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil }
	service := New(nil, nil, time.Minute, nil)
	resolve := func(sess stubSourceSession, userAgent string) string {
		t.Helper()
		specs, err := service.resolveSpecs(context.Background(), sess, "acct", "pick", userAgent)
		if err != nil || len(specs) != 1 {
			t.Fatalf("specs=%+v err=%v", specs, err)
		}
		return specs[0].address
	}

	first := stubSourceSession{download: download, play: play, authVersion: 1}
	if got := resolve(first, "Chrome"); got != "https://cdn.example/video.mp4?ua=Chrome" {
		t.Fatalf("chrome address = %q", got)
	}
	if got := resolve(first, "Safari"); got != "https://cdn.example/video.mp4?ua=Safari" {
		t.Fatalf("another user agent reused %q", got)
	}
	relogin := stubSourceSession{download: download, play: play, authVersion: 2}
	resolve(relogin, "Chrome")
	if downloads != 3 {
		t.Fatalf("DownloadURL called %d times, want 3 (user agent and authorization each miss the cache)", downloads)
	}
	resolve(first, "Chrome")
	if downloads != 3 {
		t.Fatalf("same user agent and authorization missed the cache: %d downloads", downloads)
	}

	// Expired entries are dropped when read instead of accumulating.
	service.urlMu.Lock()
	keys := make([]string, 0, len(service.urlCache))
	for key, entry := range service.urlCache {
		entry.expires = time.Now().Add(-time.Second)
		service.urlCache[key] = entry
		keys = append(keys, key)
	}
	service.urlMu.Unlock()
	for _, key := range keys {
		if _, ok := service.cachedDownloadURL(key); ok {
			t.Fatalf("expired entry %s served", key)
		}
	}
	if remaining := len(service.urlCache); remaining != 0 {
		t.Fatalf("expired entries kept: %d", remaining)
	}
	resolve(first, "Chrome")
	if downloads != 4 {
		t.Fatalf("expired URL reused: %d downloads", downloads)
	}
}
