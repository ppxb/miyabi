package playback

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestRewritePlaylistRegistersEveryResource(t *testing.T) {
	base, err := url.Parse("https://cdn.example/path/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	var resources []string
	register := func(target *url.URL, playlist bool) (string, error) {
		resources = append(resources, target.String())
		return fmt.Sprintf("/api/play/session/stream/%d", len(resources)-1), nil
	}
	body := []byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1000
variant.m3u8
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="zh",URI="audio.m3u8"
#EXTINF:4,
segment-1.ts
`)
	got, err := rewritePlaylist(body, base, register)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://cdn.example/path/variant.m3u8",
		"https://cdn.example/path/audio.m3u8",
		"https://cdn.example/path/segment-1.ts",
	}
	if len(resources) != len(want) {
		t.Fatalf("resources = %v, want %v", resources, want)
	}
	for i := range want {
		if resources[i] != want[i] {
			t.Fatalf("resource[%d] = %q, want %q", i, resources[i], want[i])
		}
	}
	for i := range want {
		local := fmt.Sprintf("/api/play/session/stream/%d", i)
		if !strings.Contains(got, local) {
			t.Fatalf("rewritten playlist missing %q:\n%s", local, got)
		}
	}
	if !strings.Contains(got, `URI="/api/play/session/stream/1"`) {
		t.Fatalf("URI attribute not rewritten:\n%s", got)
	}
}

func TestRewritePlaylistRejectsInvalidURI(t *testing.T) {
	base, _ := url.Parse("https://cdn.example/master.m3u8")
	register := func(*url.URL, bool) (string, error) { return "", nil }
	if _, err := rewritePlaylist([]byte("#EXTM3U\nhttp://[::1\n"), base, register); err == nil {
		t.Fatal("expected invalid URI error")
	}
}
