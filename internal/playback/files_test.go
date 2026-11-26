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
	info        func(context.Context, string) (pan.FileInfo, error)
	authVersion uint64
}

func (s stubSourceSession) AuthorizationVersion() uint64 { return s.authVersion }

func (s stubSourceSession) Info(ctx context.Context, fileID string) (pan.FileInfo, error) {
	if s.info != nil {
		return s.info(ctx, fileID)
	}
	return pan.FileInfo{}, nil
}

func (s stubSourceSession) DownloadURL(ctx context.Context, pickCode, userAgent string) (string, error) {
	return s.download(ctx, pickCode, userAgent)
}

func (s stubSourceSession) PlayURL(ctx context.Context, pickCode, userAgent string) ([]pan.PlaySource, error) {
	return s.play(ctx, pickCode, userAgent)
}

func (s stubSourceSession) WithSource(_ context.Context, fn func() error) error { return fn() }

func newTestService(t *testing.T) *Service {
	t.Helper()
	d, err := drive.NewWithClient(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return New(nil, d, time.Minute, nil)
}

func TestResolveSpecsPrefersDirectDownload(t *testing.T) {
	specs, err := newTestService(t).resolveSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "https://cdn.example/video.mp4", nil },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, errors.New("playback URL must not be requested when a download URL exists")
		},
	}, "acct", "file", "pick", "ua")
	if err != nil || len(specs) != 1 {
		t.Fatalf("specs=%+v err=%v", specs, err)
	}
	if specs[0].mediaType != "video/mp4" || specs[0].address != "https://cdn.example/video.mp4" || specs[0].playlist || specs[0].label != "原画" {
		t.Fatalf("unexpected direct spec: %+v", specs[0])
	}
}

func TestResolveSpecsFallsBackToHLS(t *testing.T) {
	specs, err := newTestService(t).resolveSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return []pan.PlaySource{
				{URL: "https://cdn.example/1080.m3u8", Height: 1080, Definition: 1},
				{URL: "https://cdn.example/2160.m3u8", Height: 2160, Definition: 100},
			}, nil
		},
	}, "acct", "file", "pick", "ua")
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

func TestResolveSpecsReportsDownloadErrors(t *testing.T) {
	cause := errors.New("network down")
	if _, err := newTestService(t).resolveSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", cause },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, nil
		},
	}, "acct", "file", "pick", "ua"); !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped download failure", err)
	}
}

func TestResolveSpecsReportsTranscodeUnavailable(t *testing.T) {
	_, err := newTestService(t).resolveSpecs(context.Background(), stubSourceSession{
		download: func(context.Context, string, string) (string, error) { return "", pan.ErrDownloadUnavailable },
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, pan.ErrTranscodeUnavailable
		},
	}, "acct", "file", "pick", "ua")
	if !errors.Is(err, drive.ErrTranscodeUnavailable) {
		t.Fatalf("error = %v, want transcode unavailable", err)
	}
}

func TestResolveSpecsCachesDirectURL(t *testing.T) {
	service := newTestService(t)
	downloads := 0
	sess := stubSourceSession{
		download: func(context.Context, string, string) (string, error) {
			downloads++
			return "https://cdn.example/video.mp4?t=4102444800", nil
		},
		play: func(context.Context, string, string) ([]pan.PlaySource, error) {
			return nil, errors.New("playback URL must not be requested when a download URL exists")
		},
	}
	for i := 0; i < 3; i++ {
		specs, err := service.resolveSpecs(context.Background(), sess, "acct", "file", "pick", "ua")
		if err != nil || len(specs) != 1 || specs[0].address != "https://cdn.example/video.mp4?t=4102444800" || specs[0].mediaType != "video/mp4" {
			t.Fatalf("iteration %d: specs=%+v err=%v", i, specs, err)
		}
	}
	if downloads != 1 {
		t.Fatalf("DownloadURL called %d times, want 1 (later calls served from cache)", downloads)
	}
}
