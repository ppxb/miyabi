package subtitle

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
)

const (
	simplifiedSRT  = "1\n00:00:01,000 --> 00:00:02,000\n这是一个关于开发的问题\n"
	traditionalSRT = "1\n00:00:01,000 --> 00:00:02,000\n這是一個關於開發的問題\n"
	simplifiedASS  = "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,这是\n"
)

type fixedProvider []Candidate

func (provider fixedProvider) Search(context.Context, string) ([]Candidate, error) {
	return provider, nil
}

type subtitleTransport func(*http.Request) (*http.Response, error)

func (f subtitleTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// exportFixture supplies subtitle responses without bypassing URL validation.
func exportFixture(t *testing.T, bodies map[string]string, candidates ...Candidate) (*Service, *ent.Client, int, subtitlemeta.Target) {
	t.Helper()
	for i := range candidates {
		candidates[i].URL = "http://93.184.216.34" + candidates[i].URL
	}
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	film := store.Client.Movie.Create().SetCode("ABP-123").SaveX(t.Context())
	finder := NewFinder(nil, WithProviders(fixedProvider(candidates)))
	finder.client.Transport = subtitleTransport(func(req *http.Request) (*http.Response, error) {
		body, ok := bodies[req.URL.Path]
		status := http.StatusOK
		if !ok {
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})
	target := subtitlemeta.Target{Dir: filepath.Join(t.TempDir(), "ABP", "ABP-123"), Stem: "ABP-123", Code: "ABP-123"}
	return NewService(store.Client, finder), store.Client, film.ID, target
}

func exportedFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestExportWritesDistinctOnlineSubtitlesAndReusesExports(t *testing.T) {
	service, db, movieID, target := exportFixture(t, map[string]string{
		"/same-kind.srt":   simplifiedSRT,
		"/traditional.srt": traditionalSRT,
		"/duplicate.srt":   traditionalSRT,
		"/styled.ass":      simplifiedASS,
		"/extra.vtt":       "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n这是\n",
	},
		Candidate{Name: "ABP-123.chs.srt", URL: "/same-kind.srt", Format: "srt", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard},
		Candidate{Name: "ABP-123.srt", URL: "/traditional.srt", Format: "srt", Version: subtitlemeta.VersionStandard},
		Candidate{Name: "ABP-123 copy.srt", URL: "/duplicate.srt", Format: "srt", Version: subtitlemeta.VersionStandard},
		Candidate{Name: "ABP-123.ass", URL: "/styled.ass", Format: "ass", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard},
		Candidate{Name: "ABP-123.vtt", URL: "/extra.vtt", Format: "vtt", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard},
	)
	ctx := t.Context()
	written, err := service.Export(ctx, movieID, target)
	if err != nil || written != MaxTracks {
		t.Fatalf("Export = %d, %v", written, err)
	}
	// The first online subtitle claims zh-CN SRT.
	// The unlabelled download is detected as Traditional; its byte-identical copy is skipped.
	want := []string{"ABP-123.zh-CN.ass", "ABP-123.zh-CN.srt", "ABP-123.zh-TW.srt"}
	if got := exportedFiles(t, target.Dir); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("exported files = %v; want %v", got, want)
	}
	body, err := os.ReadFile(filepath.Join(target.Dir, "ABP-123.zh-CN.srt"))
	if err != nil || string(body) != "\uFEFF"+simplifiedSRT {
		t.Fatalf("online subtitle was not exported as UTF-8: %q, %v", body, err)
	}
	if count := db.Subtitle.Query().Where(subtitle.SourceURLNEQ("")).CountX(ctx); count != MaxTracks {
		t.Fatalf("online subtitle records = %d", count)
	}

	// A second export finds every kind in place and downloads nothing.
	service.finder.client.Transport = subtitleTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("repeated export downloaded an existing subtitle")
		return nil, nil
	})
	if written, err := service.Export(ctx, movieID, target); err != nil || written != 0 {
		t.Fatalf("repeat Export = %d, %v", written, err)
	}
}

func TestExportReplacesRemovedOnlineSubtitles(t *testing.T) {
	service, db, movieID, target := exportFixture(t, map[string]string{"/first.srt": simplifiedSRT},
		Candidate{Name: "ABP-123.chs.srt", URL: "/first.srt", Format: "srt", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard})
	ctx := t.Context()
	if written, err := service.Export(ctx, movieID, target); err != nil || written != 1 {
		t.Fatalf("Export = %d, %v", written, err)
	}
	path := filepath.Join(target.Dir, "ABP-123.zh-CN.srt")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if written, err := service.Export(ctx, movieID, target); err != nil || written != 1 {
		t.Fatalf("Export after removal = %d, %v", written, err)
	}
	if !fileExists(path) || db.Subtitle.Query().CountX(ctx) != 1 {
		t.Fatalf("removed subtitle was not replaced exactly once: %v", exportedFiles(t, target.Dir))
	}
}

func TestExportSkipsOnlineSearchForHardSubtitledVideos(t *testing.T) {
	service, db, movieID, target := exportFixture(t, map[string]string{"/first.srt": simplifiedSRT},
		Candidate{Name: "ABP-123.chs.srt", URL: "/first.srt", Format: "srt", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard})
	target.HardSubtitled = true
	if written, err := service.Export(t.Context(), movieID, target); err != nil || written != 0 {
		t.Fatalf("Export = %d, %v", written, err)
	}
	if db.Subtitle.Query().CountX(t.Context()) != 0 {
		t.Fatal("hard-subtitled video fetched online subtitles")
	}
}

func TestTargetPathNamesVersionAndLanguageForEmby(t *testing.T) {
	target := subtitlemeta.Target{Dir: "emby", Stem: "ABP-123"}
	for kind, want := range map[subtitlemeta.Kind]string{
		{Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard, Format: "srt"}:    "ABP-123.zh-CN.srt",
		{Language: subtitlemeta.LangTraditionalChinese, Version: subtitlemeta.VersionUncensored, Format: "ass"}: "ABP-123.uncensored.zh-TW.ass",
	} {
		if got := target.Path(kind); got != filepath.Join("emby", want) {
			t.Errorf("Path(%+v) = %s; want %s", kind, got, want)
		}
	}
}

func TestFinderBlocksLoopbackDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(simplifiedSRT))
	}))
	defer server.Close()

	_, _, err := NewFinder(nil).Download(t.Context(), Candidate{URL: server.URL, Format: "srt"})
	if err == nil {
		t.Fatal("expected SSRF error when accessing loopback server")
	}
	if !strings.Contains(err.Error(), "prohibited") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExportDeletesOrphanPendingSubtitles(t *testing.T) {
	service, db, movieID, target := exportFixture(t, nil)
	ctx := t.Context()

	orphan := db.Subtitle.Create().
		SetMovieID(movieID).
		SetName("ABP-123.orphan.srt").
		SetFormat("srt").
		SetStoragePath(filepath.Join(target.Dir, "non-existent.srt")).
		SaveX(ctx)

	if _, err := service.Export(ctx, movieID, target); err != nil {
		t.Fatalf("Export = %v", err)
	}

	exists, err := db.Subtitle.Query().Where(subtitle.IDEQ(orphan.ID)).Exist(ctx)
	if err != nil || exists {
		t.Fatalf("expected orphan subtitle to be deleted, exists: %v, err: %v", exists, err)
	}
}
