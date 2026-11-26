package drive

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/pan"
)

type mediaSessionStub struct {
	authVersion uint64
	sourceErr   error
	download    func(context.Context, string, string) (string, error)
	play        func(context.Context, string, string) ([]pan.PlaySource, error)
	info        func(context.Context, string) (pan.FileInfo, error)
}

func (s mediaSessionStub) AuthorizationVersion() uint64 { return s.authVersion }

func (s mediaSessionStub) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	if s.info != nil {
		return s.info(ctx, fileID)
	}
	return pan.FileInfo{}, nil
}

func (s mediaSessionStub) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return s.download(ctx, pickCode, userAgent)
}

func (s mediaSessionStub) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return s.play(ctx, pickCode, userAgent)
}

func (s mediaSessionStub) WithSource(_ context.Context, fn func() error) error {
	if s.sourceErr != nil {
		return s.sourceErr
	}
	return fn()
}

func signedMediaURL(id int, expires time.Time) string {
	return fmt.Sprintf("https://cdn.example/%d.mp4?sign=a%%2Bb&t=%d", id, expires.Unix())
}

func TestResolveMediaPrefersDirectDownload(t *testing.T) {
	d := &Drive{}
	media, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		download: func(context.Context, string, string) (string, error) { return "https://cdn.example/video.mp4", nil },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, errors.New("playback URL must not be requested when a download URL exists")
		},
	}, "acct", "file", "pick", "ua")
	if err != nil || media.DownloadURL != "https://cdn.example/video.mp4" || len(media.Streams) != 0 {
		t.Fatalf("media=%+v err=%v", media, err)
	}
}

func TestResolveMediaFallsBackToStreams(t *testing.T) {
	d := &Drive{}
	media, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return []pan.PlaySource{{URL: "https://cdn.example/1080.m3u8", Height: 1080}}, nil
		},
	}, "acct", "file", "pick", "ua")
	if err != nil || media.DownloadURL != "" || len(media.Streams) != 1 || media.Streams[0].Height != 1080 {
		t.Fatalf("media=%+v err=%v", media, err)
	}
}

func TestResolveMediaReportsDownloadErrors(t *testing.T) {
	d := &Drive{}
	cause := errors.New("network down")
	_, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		download: func(context.Context, string, string) (string, error) { return "", cause },
		play:     func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil },
	}, "acct", "file", "pick", "ua")
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped download failure", err)
	}
}

func TestResolveMediaReportsTranscodeUnavailable(t *testing.T) {
	d := &Drive{}
	_, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, pan.ErrTranscodeUnavailable
		},
	}, "acct", "file", "pick", "ua")
	if !errors.Is(err, ErrTranscodeUnavailable) {
		t.Fatalf("error = %v, want transcode unavailable", err)
	}
}

func TestResolveMediaReadsMissingPickCode(t *testing.T) {
	d := &Drive{}
	var received string
	media, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		info: func(_ context.Context, fileID string) (pan.FileInfo, error) {
			if fileID != "file" {
				t.Fatalf("info file = %q", fileID)
			}
			return pan.FileInfo{PickCode: "from-info"}, nil
		},
		download: func(_ context.Context, pickCode, _ string) (string, error) {
			received = pickCode
			return "https://cdn.example/video.mp4", nil
		},
		play: func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil },
	}, "acct", "file", "", "ua")
	if err != nil || received != "from-info" || media.DownloadURL != "https://cdn.example/video.mp4" {
		t.Fatalf("pick code=%q media=%+v err=%v", received, media, err)
	}
}

func TestResolveMediaCachesDirectURL(t *testing.T) {
	d := &Drive{}
	downloads := 0
	sess := mediaSessionStub{
		download: func(context.Context, string, string) (string, error) {
			downloads++
			return "https://cdn.example/video.mp4?t=4102444800", nil
		},
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, errors.New("playback URL must not be requested when a download URL exists")
		},
	}
	for i := range 3 {
		media, err := d.ResolveMedia(context.Background(), sess, "acct", "file", "pick", "ua")
		if err != nil || media.DownloadURL != "https://cdn.example/video.mp4?t=4102444800" {
			t.Fatalf("iteration %d: media=%+v err=%v", i, media, err)
		}
	}
	if downloads != 1 {
		t.Fatalf("DownloadURL called %d times, want 1 (later calls served from cache)", downloads)
	}
}

// 115 direct URLs are bound to the requesting user agent and authorization, so
// another browser or a new login must resolve its own URL.
func TestResolveMediaCacheIsScopedToUserAgentAndAuthorization(t *testing.T) {
	d := &Drive{}
	downloads := 0
	download := func(_ context.Context, _ string, userAgent string) (string, error) {
		downloads++
		return signedMediaURL(downloads, time.Now().Add(time.Hour)) + "&ua=" + userAgent, nil
	}
	play := func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil }
	resolve := func(sess mediaSessionStub, userAgent string) string {
		t.Helper()
		media, err := d.ResolveMedia(context.Background(), sess, "acct", "file", "pick", userAgent)
		if err != nil || media.DownloadURL == "" {
			t.Fatalf("media=%+v err=%v", media, err)
		}
		return media.DownloadURL
	}

	first := mediaSessionStub{download: download, play: play, authVersion: 1}
	resolve(first, "Chrome")
	resolve(first, "Safari")
	relogin := mediaSessionStub{download: download, play: play, authVersion: 2}
	resolve(relogin, "Chrome")
	if downloads != 3 {
		t.Fatalf("DownloadURL called %d times, want 3 (user agent and authorization each miss the cache)", downloads)
	}
	resolve(first, "Chrome")
	if downloads != 3 {
		t.Fatalf("same user agent and authorization missed the cache: %d downloads", downloads)
	}

	// Expired entries are dropped when read instead of accumulating.
	d.media.mu.Lock()
	keys := make([]string, 0, len(d.media.urls))
	for key, entry := range d.media.urls {
		entry.expires = time.Now().Add(-time.Second)
		d.media.urls[key] = entry
		keys = append(keys, key)
	}
	d.media.mu.Unlock()
	for _, key := range keys {
		if media, ok := d.media.cached(key, time.Now()); ok {
			t.Fatalf("expired entry %s served: %+v", key, media)
		}
	}
	resolve(first, "Chrome")
	if downloads != 4 {
		t.Fatalf("expired URL reused: %d downloads", downloads)
	}
}

func TestResolveMediaRefreshesBeforeSignatureExpires(t *testing.T) {
	for _, lifetime := range []time.Duration{time.Hour, 45 * time.Second} {
		t.Run(lifetime.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				d := &Drive{}
				var calls int
				sess := mediaSessionStub{
					download: func(context.Context, string, string) (string, error) {
						calls++
						return signedMediaURL(calls, time.Now().Add(lifetime)), nil
					},
					play: func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil },
				}
				first, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", "")
				if err != nil {
					t.Fatal(err)
				}
				deadline := min(mediaCacheTTL, lifetime-mediaExpiryMargin)
				time.Sleep(deadline - time.Nanosecond)
				if media, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", ""); err != nil || media.DownloadURL != first.DownloadURL || calls != 1 {
					t.Fatalf("early refresh: calls=%d media=%+v err=%v", calls, media, err)
				}
				time.Sleep(time.Nanosecond)
				if media, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", ""); err != nil || media.DownloadURL == first.DownloadURL || calls != 2 {
					t.Fatalf("missing refresh: calls=%d media=%+v err=%v", calls, media, err)
				}
			})
		})
	}
}

func TestMediaCacheSkipsUncertainOrExpiringSignatures(t *testing.T) {
	now := time.Unix(1700000000, 0)
	for _, address := range []string{
		"https://cdn.example/video.mp4",
		"https://cdn.example/video.m3u8?auth_key=opaque",
		"https://cdn.example/video.mp4?t=invalid",
		"https://cdn.example/video.mp4?t=",
		"https://cdn.example/video.mp4?t=-1",
		"https://cdn.example/video.mp4?t=1700003600&expires=invalid",
		"https://cdn.example/video.mp4?t=1700003600&bad=%zz",
		signedMediaURL(1, now), signedMediaURL(1, now.Add(mediaExpiryMargin)),
	} {
		resolver := &mediaResolver{}
		resolver.remember("file", Media{DownloadURL: address}, now)
		if media, ok := resolver.cached("file", now); ok {
			t.Errorf("unsafe signature cached: %s (%+v)", address, media)
		}
	}
	address := "https://cdn.example/video.mp4?t=1700003600&expires=1700000045"
	if expires := mediaCacheExpiry(address, now); !expires.Equal(now.Add(15 * time.Second)) {
		t.Errorf("earliest signature deadline = %v", expires)
	}
}

func TestResolveMediaRevalidatesSourceOnCacheHit(t *testing.T) {
	d := &Drive{}
	downloads := 0
	download := func(context.Context, string, string) (string, error) {
		downloads++
		return signedMediaURL(1, time.Now().Add(time.Hour)), nil
	}
	play := func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil }
	if _, err := d.ResolveMedia(context.Background(), mediaSessionStub{download: download, play: play}, "acct", "file", "pick", "ua"); err != nil {
		t.Fatal(err)
	}
	_, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		download: download, play: play, sourceErr: ErrSourceChanged,
	}, "acct", "file", "pick", "ua")
	if !errors.Is(err, ErrSourceChanged) || downloads != 1 {
		t.Fatalf("cache hit source check: err=%v downloads=%d", err, downloads)
	}
}

func TestResolveMediaReportsInfoErrors(t *testing.T) {
	cause := errors.New("info failed")
	d := &Drive{}
	_, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		info:     func(context.Context, string) (pan.FileInfo, error) { return pan.FileInfo{}, cause },
		download: func(context.Context, string, string) (string, error) { return "", nil },
		play:     func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil },
	}, "acct", "file", "", "ua")
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped info failure", err)
	}
}

func TestResolveMediaRejectsMissingPickCode(t *testing.T) {
	d := &Drive{}
	_, err := d.ResolveMedia(context.Background(), mediaSessionStub{
		info:     func(context.Context, string) (pan.FileInfo, error) { return pan.FileInfo{}, nil },
		download: func(context.Context, string, string) (string, error) { return "", nil },
		play:     func(context.Context, string, string) ([]pan.PlaySource, error) { return nil, nil },
	}, "acct", "file", "", "ua")
	if !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("error = %v, want KindNotFound", err)
	}
}

func TestResolveMediaCachesStreamsUntilEarliestExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := &Drive{}
		plays := 0
		sess := mediaSessionStub{
			download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
			play: func(context.Context, string, string) ([]pan.PlaySource, error) {
				plays++
				return []pan.PlaySource{
					{URL: signedMediaURL(1, time.Now().Add(40*time.Second)), Height: 1080},
					{URL: signedMediaURL(2, time.Now().Add(70*time.Second)), Height: 2160},
				}, nil
			},
		}
		if _, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", "ua"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(9 * time.Second)
		if media, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", "ua"); err != nil || plays != 1 || len(media.Streams) != 2 {
			t.Fatalf("cache hit: plays=%d media=%+v err=%v", plays, media, err)
		}
		time.Sleep(2 * time.Second)
		if _, err := d.ResolveMedia(t.Context(), sess, "acct", "file", "pick", "ua"); err != nil || plays != 2 {
			t.Fatalf("earliest expiry: plays=%d err=%v", plays, err)
		}
	})
}

func TestResolveMediaSkipsStreamsWithUnknownExpiry(t *testing.T) {
	d := &Drive{}
	plays := 0
	sess := mediaSessionStub{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			plays++
			return []pan.PlaySource{
				{URL: signedMediaURL(1, time.Now().Add(time.Hour)), Height: 1080},
				{URL: "https://cdn.example/unsigned.m3u8", Height: 720},
			}, nil
		},
	}
	for range 2 {
		if _, err := d.ResolveMedia(context.Background(), sess, "acct", "file", "pick", "ua"); err != nil {
			t.Fatal(err)
		}
	}
	if plays != 2 {
		t.Fatalf("streams with an unknown expiry were cached: plays=%d", plays)
	}
}

func TestMediaCacheBoundsAndReclaimsEntries(t *testing.T) {
	resolver := &mediaResolver{}
	now := time.Unix(1700000000, 0)
	for i := range mediaCacheLimit {
		resolver.remember(fmt.Sprint(i), Media{DownloadURL: signedMediaURL(i, now.Add(time.Hour))}, now.Add(time.Duration(i)*time.Millisecond))
	}
	resolver.remember("new", Media{DownloadURL: signedMediaURL(999, now.Add(time.Hour))}, now.Add(time.Second))
	first, firstOK := resolver.cached("0", now.Add(time.Second))
	fresh, freshOK := resolver.cached("new", now.Add(time.Second))
	if len(resolver.urls) != mediaCacheLimit || firstOK || !freshOK || fresh.DownloadURL == "" {
		t.Fatalf("cache did not evict the earliest deadline at capacity: %+v %v", first, firstOK)
	}
	if media, ok := resolver.cached("missing", now.Add(2*mediaCacheTTL)); ok || len(resolver.urls) != 0 {
		t.Fatalf("access did not reclaim expired entries: %+v", media)
	}
	resolver.remember("old", Media{DownloadURL: signedMediaURL(1, now.Add(time.Hour))}, now)
	resolver.remember("new", Media{DownloadURL: signedMediaURL(2, now.Add(time.Hour))}, now.Add(2*mediaCacheTTL))
	if len(resolver.urls) != 1 {
		t.Fatal("write did not reclaim expired entries")
	}
}
