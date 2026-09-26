package strm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestStreamURLPrefersOriginalQuality(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	client.playURL = func(_ context.Context, _, pickCode string) ([]pan.PlaySource, error) {
		if pickCode != "pick-101" {
			t.Fatalf("unexpected pick code: %s", pickCode)
		}
		return []pan.PlaySource{
			{URL: "https://cdn.example/1080p.m3u8", Height: 1080, Definition: 3},
			{URL: "https://cdn.example/original.mp4", Height: 720, Definition: 100},
			{URL: "https://cdn.example/480p.m3u8", Height: 480, Definition: 2},
		}, nil
	}
	got, err := relay.StreamURL(t.Context(), "101", "")
	if err != nil || got != "https://cdn.example/original.mp4" {
		t.Fatalf("StreamURL = %q, %v; want the original stream", got, err)
	}
}

func TestStreamURLAsks115WhenThePickCodeWasNotIndexed(t *testing.T) {
	relay, client := relayFixture(t, "")
	client.info = func(_ context.Context, _, id string) (pan.FileInfo, error) {
		return pan.FileInfo{File: pan.File{ID: id, Name: "ABP-001.mp4", PickCode: "info-pick"}}, nil
	}
	client.playURL = func(_ context.Context, _, pickCode string) ([]pan.PlaySource, error) {
		if pickCode != "info-pick" {
			t.Fatalf("unexpected pick code: %s", pickCode)
		}
		return []pan.PlaySource{{URL: "https://cdn.example/video.m3u8", Height: 1080, Definition: 3}}, nil
	}
	got, err := relay.StreamURL(t.Context(), "101", "")
	if err != nil || got != "https://cdn.example/video.m3u8" {
		t.Fatalf("StreamURL = %q, %v", got, err)
	}
}

func TestStreamURLReportsMissingStreams(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	client.playURL = func(context.Context, string, string) ([]pan.PlaySource, error) {
		return []pan.PlaySource{{Height: 1080}}, nil
	}
	if _, err := relay.StreamURL(t.Context(), "101", ""); err == nil {
		t.Fatal("a source list without URLs resolved to a stream")
	}
	client.playURL = func(context.Context, string, string) ([]pan.PlaySource, error) {
		return nil, pan.ErrTranscodeUnavailable
	}
	if _, err := relay.StreamURL(t.Context(), "101", ""); !errors.Is(err, drive.ErrTranscodeUnavailable) {
		t.Fatalf("missing transcodes = %v, want drive.ErrTranscodeUnavailable", err)
	}
}

func TestProbeSendsHEADToTheCDN(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	client.openMedia = func(_ context.Context, method, address string, headers http.Header) (*http.Response, error) {
		if method != http.MethodHead || address != "https://cdn.example/video.mp4" || headers.Get("Range") != "bytes=0-1" {
			t.Fatalf("unexpected probe: %s %s %v", method, address, headers)
		}
		header := http.Header{"Content-Type": {"video/mp4"}}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	response, err := relay.Probe(t.Context(), "https://cdn.example/video.mp4", http.Header{"Range": {"bytes=0-1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("unexpected probe response: %+v", response)
	}
}

func TestStreamURLPrefersDownloadURL(t *testing.T) {
	relay, client := relayFixture(t, "pick-101")
	client.downloadURL = func(_ context.Context, _, pickCode, userAgent string) (string, error) {
		if pickCode != "pick-101" {
			t.Fatalf("unexpected pick code: %s", pickCode)
		}
		if userAgent != "VidHub/1.0" {
			t.Fatalf("unexpected userAgent: %s", userAgent)
		}
		return "https://cdn.example/raw-download.mp4", nil
	}
	client.playURL = func(context.Context, string, string) ([]pan.PlaySource, error) {
		t.Fatal("PlayURL should not be called when DownloadURL succeeds")
		return nil, nil
	}
	got, err := relay.StreamURL(t.Context(), "101", "VidHub/1.0")
	if err != nil || got != "https://cdn.example/raw-download.mp4" {
		t.Fatalf("StreamURL = %q, %v; want raw download URL", got, err)
	}
}
