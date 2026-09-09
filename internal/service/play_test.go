package service

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/pan"
)

func playFixture(t *testing.T) (*PlayService, LibrarySource) {
	t.Helper()
	library, queued, payload := libraryFixture(t)
	if err := library.indexScanPage(t.Context(), queued.ID, "fixture", "/Movies", []scanVideo{
		fixtureVideo("101", "ABP-001-CD1.mp4"), fixtureVideo("102", "ABP-001-CD2.mkv"),
	}, &payload); err != nil {
		t.Fatal(err)
	}
	library.drive = &PanService{
		client:    pan.New(pan.Options{}),
		tokens:    pan.Tokens{AccessToken: "fixture-token", ExpiresAt: time.Now().Add(time.Hour)},
		directory: panLibraryDirectory{AccountID: payload.Source.AccountID, PanLibraryDirectory: payload.Source.Directory},
	}
	t.Cleanup(library.drive.Close)
	service := NewPlayService(library)
	t.Cleanup(service.Close)
	return service, payload.Source
}

func TestPlayFilesUsesOnlyCurrentLibrarySource(t *testing.T) {
	service, source := playFixture(t)
	db := service.library.database
	movieID := db.Movie.Query().Where(movie.CodeEQ("ABP-001")).OnlyIDX(t.Context())
	db.File.Create().SetFileID("201").SetName("ABP-001-other.mp4").SetSize(1).
		SetAccountID(source.AccountID).SetRootID("other").SetMovieID(movieID).SaveX(t.Context())
	db.File.Create().SetFileID("301").SetName("ABP-001-account.mp4").SetSize(1).
		SetAccountID("other").SetRootID(source.Directory.ID).SetMovieID(movieID).SaveX(t.Context())
	files, err := service.Files(t.Context(), "abp-001")
	if err != nil || len(files.Files) != 2 || files.Files[0].ID != "101" || files.Files[1].ID != "102" {
		t.Fatalf("playable files = %#v, error = %v", files, err)
	}
	if err := saveSetting(t.Context(), db, panDirectorySetting, panLibraryDirectory{
		AccountID: source.AccountID, PanLibraryDirectory: PanLibraryDirectory{ID: "empty"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Files(t.Context(), "ABP-001"); !ent.IsNotFound(err) {
		t.Fatalf("old source remains playable: %v", err)
	}
}

func TestPlayFilesPrefersLargestVideo(t *testing.T) {
	service, _ := playFixture(t)
	service.library.database.File.Update().Where(file.FileIDEQ("102")).SetSize(4096).SaveX(t.Context())

	files, err := service.Files(t.Context(), "ABP-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Files) != 2 || files.Files[0].ID != "102" || files.Files[1].ID != "101" {
		t.Fatalf("playable files are not ordered by size: %#v", files.Files)
	}
}

func TestPlaybackRejectsSourceChangesAndUnknownResources(t *testing.T) {
	for _, change := range []string{"login", "logout", "directory", "account", "release"} {
		t.Run(change, func(t *testing.T) {
			service, source := playFixture(t)
			playback, err := service.createSession(source, 0, []pan.PlaySource{{URL: "https://cdn.example/playlist?sign=fixture", Height: 1080}})
			if err != nil {
				t.Fatal(err)
			}
			session, _, err := service.resource(playback.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := service.resource(playback.ID, 99); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("unregistered resource error = %v", err)
			}
			drive := service.library.drive
			switch change {
			case "login":
				drive.authorizationVersion++
			case "logout":
				drive.tokens.AccessToken = ""
			case "directory":
				drive.directory.ID = "other"
			case "account":
				drive.directory.AccountID = "other"
			case "release":
				service.Release(playback.ID)
			}
			if _, _, err := service.resource(playback.ID, 0); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("stale playback remains available: %v", err)
			}
			if session.ctx.Err() == nil {
				t.Fatal("invalidated session did not cancel ongoing transfers")
			}
		})
	}
}

func TestPlaylistRewritesVariantsKeysMapsAndParts(t *testing.T) {
	base, _ := url.Parse("https://cdn.example/path/master?signature=fixture")
	session := &playSession{id: "fixture", byURL: make(map[string]int)}
	body := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="Main, Stereo",URI="audio?lang=zh"
#EXT-X-STREAM-INF:BANDWIDTH=1000000
video?quality=hd
#EXT-X-KEY:METHOD=AES-128,URI="../key?signature=key-fixture",IV=0x1234
#EXT-X-MAP:URI="init.mp4",BYTERANGE="100@0"
#EXT-X-PART:DURATION=0.5,URI="part.m4s?offset=1"
#EXT-X-PRELOAD-HINT:TYPE=PART,URI="part.m4s?offset=2"
#EXT-X-BYTERANGE:20@100
segment.ts?signature=segment-fixture
segment.ts?signature=segment-fixture
#EXT-X-ENDLIST
`
	rewritten, err := rewritePlaylist([]byte(body), base, session.register)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rewritten, "signature=") || strings.Contains(rewritten, "cdn.example") || !strings.Contains(rewritten, `BYTERANGE="100@0"`) || !strings.Contains(rewritten, "#EXT-X-BYTERANGE:20@100") {
		t.Fatalf("playlist leaked URLs or changed media metadata: %s", rewritten)
	}
	want := []string{
		"https://cdn.example/path/audio?lang=zh", "https://cdn.example/path/video?quality=hd",
		"https://cdn.example/key?signature=key-fixture", "https://cdn.example/path/init.mp4",
		"https://cdn.example/path/part.m4s?offset=1", "https://cdn.example/path/part.m4s?offset=2",
		"https://cdn.example/path/segment.ts?signature=segment-fixture",
	}
	if len(session.resources) != len(want) {
		t.Fatalf("registered %d resources, want %d (duplicate segments must be reused)", len(session.resources), len(want))
	}
	for i, resource := range session.resources {
		if resource.url.String() != want[i] || resource.playlist != (i < 2) {
			t.Errorf("resource %d = %v, playlist %t", i, resource.url, resource.playlist)
		}
	}
	for _, bad := range []string{"file:///private", "https://user:password@cdn.example/video"} {
		if _, err := rewritePlaylist([]byte("#EXTM3U\n"+bad), base, session.register); err == nil {
			t.Errorf("accepted unsupported URI %q", bad)
		}
	}
}

func TestPlaybackStreamPreservesRangeAndHead(t *testing.T) {
	content := "0123456789fixture-video"
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/playlist" {
			io.WriteString(writer, "#EXTM3U\n#EXTINF:5,\nvideo\n#EXT-X-ENDLIST\n")
			return
		}
		writer.Header().Set("ETag", `"fixture"`)
		http.ServeContent(writer, request, "video.mp4", time.Unix(100, 0), strings.NewReader(content))
	}))
	defer upstream.Close()
	service, source := playFixture(t)
	playback, err := service.createSession(source, 0, []pan.PlaySource{{URL: upstream.URL + "/playlist", Height: 1080}})
	if err != nil {
		t.Fatal(err)
	}
	playlist, err := service.Stream(t.Context(), playback.ID, 0, http.MethodGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	playlist.Body.Close()
	for _, test := range []struct {
		method, byteRange, ifRange, contentRange, body string
		status                                         int
	}{
		{method: "GET", body: content, status: 200},
		{method: "HEAD", byteRange: "bytes=2-5", contentRange: "bytes 2-5/23", status: 206},
		{method: "GET", byteRange: "bytes=2-5", ifRange: `"fixture"`, contentRange: "bytes 2-5/23", body: "2345", status: 206},
		{method: "GET", byteRange: "bytes=2-5", ifRange: `"changed"`, body: content, status: 200},
		{method: "GET", byteRange: "bytes=1000-", contentRange: "bytes */23", status: 416},
	} {
		response, err := service.Stream(t.Context(), playback.ID, 1, test.method, http.Header{"Range": {test.byteRange}, "If-Range": {test.ifRange}})
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status || string(body) != test.body || response.Header.Get("Content-Range") != test.contentRange {
			t.Errorf("%s %s: status=%d range=%q body=%q error=%v", test.method, test.byteRange, response.StatusCode, response.Header.Get("Content-Range"), body, err)
		}
	}
}

func TestPlaybackRewritesRedirectedExtensionlessPlaylists(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			http.Redirect(writer, request, "/nested/manifest?signature=fixture", http.StatusFound)
		case "/nested/manifest":
			writer.Header().Set("Content-Type", "application/octet-stream")
			writer.Header().Set("ETag", `"upstream"`)
			io.WriteString(writer, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\nmedia?quality=hd\n")
		case "/nested/media":
			io.WriteString(writer, "#EXTM3U\n#EXTINF:5,\nsegment?signature=fixture\n#EXT-X-ENDLIST\n")
		default:
			t.Errorf("unexpected upstream path: %s", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	service, source := playFixture(t)
	playback, err := service.createSession(source, 0, []pan.PlaySource{{URL: upstream.URL + "/start", Height: 1080}})
	if err != nil {
		t.Fatal(err)
	}
	if len(playback.Sources) != 1 || playback.Sources[0].Type != "application/x-mpegurl" || playback.Sources[0].Label != "1080p" {
		t.Fatalf("unexpected HLS source: %#v", playback.Sources)
	}
	for _, index := range []int{0, 1} {
		response, err := service.Stream(t.Context(), playback.ID, index, "GET", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || strings.Contains(string(body), "signature=") || !strings.Contains(string(body), "/api/play/"+playback.ID+"/stream/") {
			t.Fatalf("bad rewritten playlist %q: %v", body, err)
		}
		if response.Header.Get("Content-Length") != strconv.Itoa(len(body)) || response.Header.Get("ETag") != "" {
			t.Fatalf("rewritten representation kept upstream headers: %#v", response.Header)
		}
	}
}

func TestPlaybackReleaseCancelsStreamingWithoutBufferingWholeSegment(t *testing.T) {
	finished := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/playlist" {
			io.WriteString(writer, "#EXTM3U\n#EXTINF:5,\nvideo\n#EXT-X-ENDLIST\n")
			return
		}
		defer close(finished)
		io.WriteString(writer, "video-prefix")
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer upstream.Close()
	service, source := playFixture(t)
	playback, err := service.createSession(source, 0, []pan.PlaySource{{URL: upstream.URL + "/playlist", Height: 1080}})
	if err != nil {
		t.Fatal(err)
	}
	playlist, err := service.Stream(t.Context(), playback.ID, 0, http.MethodGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	playlist.Body.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	response, err := service.Stream(ctx, playback.ID, 1, "GET", nil)
	if err != nil {
		t.Fatalf("stream waited for the full video: %v", err)
	}
	defer response.Body.Close()
	service.Release(playback.ID)
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("closing playback did not cancel the upstream request")
	}
}
