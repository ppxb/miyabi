package subtitle

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/nfo"
)

const (
	simplifiedSRT  = "1\n00:00:01,000 --> 00:00:02,000\n这是一个关于开发的问题\n"
	traditionalSRT = "1\n00:00:01,000 --> 00:00:02,000\n這是一個關於開發的問題\n\n2\n01:59:58,500 --> 02:00:00,000\n第二行\n"
	simplifiedASS  = "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.50,0:00:02.00,Default,,0,0,0,,{\\an8}这是\\N第一句\n"
	publicHost     = "http://93.184.216.34"
)

type fixedProvider []Candidate

func (provider fixedProvider) Search(context.Context, string) ([]Candidate, error) {
	return provider, nil
}

type subtitleTransport func(*http.Request) (*http.Response, error)

func (f subtitleTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type countingNotifier struct{ dirs []string }

func (n *countingNotifier) NotifyUpdated(_ context.Context, dir string) error {
	n.dirs = append(n.dirs, dir)
	return nil
}

type subtitleFixture struct {
	service  *Service
	db       *ent.Client
	movieID  int
	dataDir  string
	export   string
	notifier *countingNotifier
}

// newSubtitleFixture serves bodies by URL path without bypassing URL validation.
func newSubtitleFixture(t *testing.T, bodies map[string]string, candidates ...Candidate) subtitleFixture {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	film := store.Client.Movie.Create().SetCode("ABP-123").SetMetadata(&nfo.Movie{Runtime: 120}).SaveX(t.Context())
	finder := NewFinder(nil, WithProviders(fixedProvider(candidates)))
	finder.client.Transport = subtitleTransport(func(req *http.Request) (*http.Response, error) {
		body, ok := bodies[req.URL.Path]
		status := http.StatusOK
		if !ok {
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})
	f := subtitleFixture{db: store.Client, movieID: film.ID, dataDir: t.TempDir(), export: t.TempDir(), notifier: &countingNotifier{}}
	f.service = NewService(store.Client, finder, f.dataDir, func() string { return f.export }, f.notifier)
	return f
}

// exportSTRM creates the single .strm a movie exported as one part has.
func (f subtitleFixture) exportSTRM(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(f.export, "ABP", "ABP-123")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ABP-123.strm"), []byte("http://relay"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func offerFor(path, format, language string) Offer {
	return Offer{Provider: "test", Name: "ABP-123", URL: publicHost + path, Format: format, Language: language}
}

func TestSaveStoresTheTrackExportsACopyAndRejectsTheSameKind(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/tw.srt": traditionalSRT, "/again.srt": traditionalSRT})
	exportDir := f.exportSTRM(t)
	ctx := t.Context()

	// An unlabelled download is detected as Traditional Chinese from its text.
	track, err := f.service.Save(ctx, f.movieID, offerFor("/tw.srt", "srt", ""))
	if err != nil || track.Language != "zh-TW" || !track.Removable {
		t.Fatalf("Save = %+v, %v", track, err)
	}
	stored := filepath.Join(f.dataDir, strconv.Itoa(f.movieID), "ABP-123.zh-TW.srt")
	body, err := os.ReadFile(stored)
	if err != nil || string(body) != string(utf8BOM)+traditionalSRT {
		t.Fatalf("stored subtitle = %q, %v", body, err)
	}
	if copied, err := os.ReadFile(filepath.Join(exportDir, "ABP-123.zh-TW.srt")); err != nil || string(copied) != string(body) {
		t.Fatalf("exported copy = %q, %v", copied, err)
	}
	if len(f.notifier.dirs) != 1 || f.notifier.dirs[0] != exportDir {
		t.Fatalf("notifications = %v", f.notifier.dirs)
	}

	_, err = f.service.Save(ctx, f.movieID, offerFor("/again.srt", "srt", "zh-TW"))
	if domain.KindOf(err) != domain.KindConflict {
		t.Fatalf("saving the same kind again = %v, want conflict", err)
	}
	if tracks, err := f.service.List(ctx, f.movieID); err != nil || len(tracks) != 1 {
		t.Fatalf("List = %+v, %v", tracks, err)
	}
}

func TestConcurrentSavesOfTheSameKindKeepOneTrack(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/a.srt": simplifiedSRT, "/b.srt": simplifiedSRT})
	f.exportSTRM(t)
	errs := make(chan error, 2)
	for _, path := range []string{"/a.srt", "/b.srt"} {
		go func() {
			_, err := f.service.Save(t.Context(), f.movieID, offerFor(path, "srt", "zh-CN"))
			errs <- err
		}()
	}
	conflicts := 0
	for range 2 {
		if err := <-errs; domain.KindOf(err) == domain.KindConflict {
			conflicts++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if count := f.db.Subtitle.Query().CountX(t.Context()); conflicts != 1 || count != 1 {
		t.Fatalf("conflicts = %d, tracks = %d; want exactly one saved track", conflicts, count)
	}
}

func TestSaveWithoutASingleSTRMKeepsOnlyTheDataCopy(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/en.srt": simplifiedSRT})
	if _, err := f.service.Save(t.Context(), f.movieID, offerFor("/en.srt", "srt", "en")); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(f.dataDir, strconv.Itoa(f.movieID), "ABP-123.en.srt")) {
		t.Fatal("provider language label was not kept")
	}
	if entries, _ := os.ReadDir(f.export); len(entries) != 0 || len(f.notifier.dirs) != 0 {
		t.Fatalf("unexported movie got export files %v or notifications %v", entries, f.notifier.dirs)
	}
}

func TestDeleteRemovesDownloadedTracksAndKeepsLocalOnes(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/cn.srt": simplifiedSRT})
	exportDir := f.exportSTRM(t)
	ctx := t.Context()
	track, err := f.service.Save(ctx, f.movieID, offerFor("/cn.srt", "srt", "zh-CN"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Delete(ctx, f.movieID, track.ID); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(f.dataDir, strconv.Itoa(f.movieID), "ABP-123.zh-CN.srt"), filepath.Join(exportDir, "ABP-123.zh-CN.srt")} {
		if fileExists(name) {
			t.Fatalf("%s survived Delete", name)
		}
	}

	local := f.db.Subtitle.Create().SetMovieID(f.movieID).SetName("ABP-123.srt").SetStoragePath(filepath.Join(exportDir, "ABP-123.srt")).SaveX(ctx)
	if err := f.service.Delete(ctx, f.movieID, local.ID); domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("deleting a local subtitle = %v, want invalid", err)
	}
	if err := f.service.Delete(ctx, f.movieID, local.ID+100); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("deleting an unknown subtitle = %v, want not found", err)
	}
}

func TestSyncExportCopiesTracksOnceTheMovieIsExported(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/cn.srt": simplifiedSRT})
	ctx := t.Context()
	if _, err := f.service.Save(ctx, f.movieID, offerFor("/cn.srt", "srt", "zh-CN")); err != nil {
		t.Fatal(err)
	}
	exportDir := f.exportSTRM(t)
	if err := f.service.SyncExport(ctx, f.movieID); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(exportDir, "ABP-123.zh-CN.srt")) || len(f.notifier.dirs) != 1 {
		t.Fatalf("SyncExport did not export the track, notifications %v", f.notifier.dirs)
	}
	// An up-to-date copy is neither rewritten nor announced again.
	if err := f.service.SyncExport(ctx, f.movieID); err != nil || len(f.notifier.dirs) != 1 {
		t.Fatalf("repeat SyncExport = %v, notifications %v", err, f.notifier.dirs)
	}
}

func TestPreviewSummarizesCuesAgainstTheRuntime(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/tw.srt": traditionalSRT, "/styled.ass": simplifiedASS})
	ctx := t.Context()
	preview, err := f.service.Preview(ctx, f.movieID, offerFor("/tw.srt", "srt", ""))
	if err != nil || preview.Cues != 2 || preview.End != 7200 || preview.Runtime != 120 || preview.Language != "zh-TW" {
		t.Fatalf("SRT preview = %+v, %v", preview, err)
	}
	if preview.Lines[0].Start != 1 || preview.Lines[0].Text != "這是一個關於開發的問題" {
		t.Fatalf("SRT first line = %+v", preview.Lines[0])
	}
	styled, err := f.service.Preview(ctx, f.movieID, offerFor("/styled.ass", "ass", "zh-CN"))
	if err != nil || styled.Cues != 1 || styled.Lines[0].Text != "这是 第一句" || styled.Lines[0].Start != 1.5 {
		t.Fatalf("ASS preview = %+v, %v", styled, err)
	}
	if tracks, _ := f.service.List(ctx, f.movieID); len(tracks) != 0 {
		t.Fatal("Preview saved a track")
	}
	if _, err := f.service.Preview(ctx, f.movieID, offerFor("/missing.srt", "srt", "")); domain.KindOf(err) != domain.KindUpstream {
		t.Fatalf("missing subtitle preview = %v, want upstream", err)
	}
}

func TestSearchReturnsRankedOffersForTheMovie(t *testing.T) {
	f := newSubtitleFixture(t, nil,
		Candidate{Provider: "A", Name: "ABP-123.srt", URL: publicHost + "/a.srt", Format: "srt", Language: subtitlemeta.LangTraditionalChinese, Version: subtitlemeta.VersionStandard},
		Candidate{Provider: "B", Name: "ABP-123.chs.srt", URL: publicHost + "/b.srt", Format: "srt", Language: subtitlemeta.LangSimplifiedChinese, Version: subtitlemeta.VersionStandard},
		Candidate{Provider: "C", Name: "ABP-999.srt", URL: publicHost + "/c.srt", Format: "srt", Language: subtitlemeta.LangSimplifiedChinese},
		Candidate{Provider: "D", Name: "ABP-123 leaked.srt", URL: publicHost + "/d.srt", Format: "srt", Version: subtitlemeta.VersionLeaked},
	)
	offers, err := f.service.Search(t.Context(), f.movieID)
	if err != nil || len(offers) != 2 || offers[0].Provider != "B" || offers[1].Provider != "A" {
		t.Fatalf("Search = %+v, %v", offers, err)
	}
	if _, err := f.service.Search(t.Context(), f.movieID+100); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("unknown movie search = %v", err)
	}
}

func TestCuesReadsDownloadedAndLocalTracksForThePlayer(t *testing.T) {
	f := newSubtitleFixture(t, map[string]string{"/styled.ass": simplifiedASS})
	ctx := t.Context()
	saved, err := f.service.Save(ctx, f.movieID, offerFor("/styled.ass", "ass", "zh-CN"))
	if err != nil {
		t.Fatal(err)
	}
	cues, err := f.service.Cues(ctx, f.movieID, saved.ID)
	if err != nil || len(cues) != 1 || cues[0].StartTime != 1.5 || cues[0].EndTime != 2 || cues[0].Text != "这是\n第一句" {
		t.Fatalf("ASS cues = %+v, %v", cues, err)
	}

	// A local subtitle in GBK: "你好" encoded as C4 E3 BA C3.
	local := filepath.Join(t.TempDir(), "ABP-123.srt")
	if err := os.WriteFile(local, append([]byte("1\n00:00:03,000 --> 00:00:04,000\n"), 0xC4, 0xE3, 0xBA, 0xC3, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	record := f.db.Subtitle.Create().SetMovieID(f.movieID).SetName("ABP-123.srt").SetStoragePath(local).SaveX(ctx)
	cues, err = f.service.Cues(ctx, f.movieID, record.ID)
	if err != nil || len(cues) != 1 || cues[0].Text != "你好" || cues[0].StartTime != 3 {
		t.Fatalf("GBK cues = %+v, %v", cues, err)
	}
	if _, err := f.service.Cues(ctx, f.movieID+1, record.ID); domain.KindOf(err) != domain.KindNotFound {
		t.Fatalf("another movie's track = %v, want not found", err)
	}
}

func TestDownloadRejectsUnsupportedFormats(t *testing.T) {
	f := newSubtitleFixture(t, nil)
	_, err := f.service.Save(t.Context(), f.movieID, offerFor("/x.sub", "sub", ""))
	if domain.KindOf(err) != domain.KindInvalid || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported format = %v, want invalid", err)
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
