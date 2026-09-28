package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanCombinesPartsPreservesMetadataAndRetainsUnmatchedFiles(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	metadata, err := lib.database.Movie.Create().SetCode("ABP-001").SetTitle("Existing title").
		SetJavdbID("fixture").SetScrapeStatus(movie.ScrapeStatusDone).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	videos := []scan.Video{
		fixtureVideo("101", "abp001-CD1.mp4"),
		fixtureVideo("102", "ABP-001-CD2.mkv"),
		fixtureVideo("103", "recording.mp4"),
	}
	for _, marker := range []string{"first", "second"} {
		if err := indexScanPage(ctx, lib, queued.ID, marker, "/Movies", videos, &payload); err != nil {
			t.Fatal(err)
		}
	}
	page, err := lib.Movies(ctx, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Movies) != 1 || lib.database.File.Query().CountX(ctx) != 3 ||
		lib.database.File.Query().Where(file.MovieIDIsNil()).CountX(ctx) != 1 {
		t.Fatalf("library = %#v", page)
	}
	item := page.Movies[0]
	if item.ID != metadata.ID || item.Title != metadata.Title || lib.database.Movie.GetX(ctx, item.ID).ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("scanned movie = %#v", item)
	}
	// A renamed video that no longer identifies a code must not keep a stale
	// association, while the other part still keeps the movie in the library.
	if err := indexScanPage(ctx, lib, queued.ID, "third", "/Movies", []scan.Video{fixtureVideo("102", "recording-two.mkv")}, &payload); err != nil {
		t.Fatal(err)
	}
	renamed, err := lib.database.File.Query().Where(file.FileIDEQ("102")).Only(ctx)
	if err != nil || renamed.MovieID != nil {
		t.Fatalf("renamed video = %#v, error = %v", renamed, err)
	}
	remaining, err := lib.database.Movie.Get(ctx, metadata.ID)
	if err != nil || remaining.JavdbID == nil || *remaining.JavdbID != "fixture" {
		t.Fatalf("remaining metadata = %#v, error = %v", remaining, err)
	}
}

func TestScanKeepsLetterSerialsDistinctAndDoesNotExtractPartialNumbers(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := indexScanPage(ctx, lib, queued.ID, "fixture", "/Movies", []scan.Video{
		fixtureVideo("letter", "KNB-M014.mp4"),
		fixtureVideo("short", "M-014.mp4"),
		fixtureVideo("unidentified", "UNKNOWN-KNB-M014.mp4"),
	}, &payload); err != nil {
		t.Fatal(err)
	}
	page, err := lib.Movies(ctx, 1, 24)
	if err != nil || page.Total != 2 || lib.database.File.Query().Where(file.MovieIDIsNil()).CountX(ctx) != 1 {
		t.Fatalf("letter serials were lost or merged: %#v, %v", page, err)
	}
	ids := make(map[string]int)
	for _, item := range page.Movies {
		ids[item.Code] = item.ID
	}
	if ids["KNB-M014"] == 0 || ids["M-014"] == 0 || ids["KNB-M014"] == ids["M-014"] {
		t.Fatalf("incorrect catalogue identities: %#v", ids)
	}
	unknown := lib.database.File.Query().Where(file.FileIDEQ("unidentified")).OnlyX(ctx)
	if unknown.MovieID != nil {
		t.Fatal("unrecognized filename was associated with a partial catalogue number")
	}
}

func TestScanCombinesCatalogueAliasesAndReplacesFailedLegacyIndex(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	legacy := lib.database.Movie.Create().SetCode("259LUXU-1899").
		SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
	lib.database.File.Create().SetFileID("prefixed").SetName("259LUXU-1899.mp4").SetSize(1 << 30).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(legacy).SaveX(ctx)
	known := lib.database.Movie.Create().SetCode("LUXU-1899").SetTitle("Catalogue title").
		SetJavdbID("catalogue-id").SetScrapeStatus(movie.ScrapeStatusDone).SaveX(ctx)
	videos := []scan.Video{
		fixtureVideo("prefixed", "259LUXU-1899.mp4"),
		fixtureVideo("catalogue", "LUXU-1899-CD2.mkv"),
	}
	for _, marker := range []string{"first", "rescan"} {
		if err := indexScanPage(ctx, lib, queued.ID, marker, "/Movies", videos, &payload); err != nil {
			t.Fatal(err)
		}
	}
	page, err := lib.Movies(ctx, 1, 24)
	if err != nil || page.Total != 1 || len(page.Movies) != 1 {
		t.Fatalf("catalogue aliases created duplicate movies: %#v, %v", page, err)
	}
	item := page.Movies[0]
	if item.ID != known.ID || item.Code != "LUXU-1899" || lib.database.File.Query().CountX(ctx) != 2 ||
		item.Title != known.Title || lib.database.Movie.GetX(ctx, item.ID).ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("alias scan lost existing metadata or file associations: %#v", item)
	}
	if _, err := lib.database.Movie.Get(ctx, legacy.ID); !ent.IsNotFound(err) {
		t.Fatalf("unreferenced legacy alias was not removed: %v", err)
	}
}

func TestMetadataCanonicalizesLegacyAliasBeforeRescan(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	legacy := lib.database.Movie.Create().SetCode("259LUXU-1899").SaveX(ctx)
	lib.database.File.Create().SetFileID("prefixed").SetName("259LUXU-1899.mp4").SetSize(1 << 30).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(legacy).SaveX(ctx)
	doc := nfo.Movie{Code: "LUXU-1899", Title: "Catalogue title",
		IDs: []nfo.UniqueID{{Type: "javdb", Default: true, Value: "catalogue-id"}},
	}
	if err := ent.WithTx(ctx, lib.database, func(tx *ent.Tx) error {
		return scrapePkg.SaveMovieMetadata(ctx, tx, legacy.ID, doc)
	}); err != nil {
		t.Fatal(err)
	}
	if err := indexScanPage(ctx, lib, queued.ID, "rescan", "/Movies",
		[]scan.Video{fixtureVideo("prefixed", "259LUXU-1899.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	record := lib.database.Movie.Query().OnlyX(ctx)
	if record.ID != legacy.ID || record.Code != doc.Code || record.Title != doc.Title || domain.ValueOrZero(record.JavdbID) != doc.JavDBID() {
		t.Fatalf("metadata normalization changed the movie identity on rescan: %#v", record)
	}
}

func TestOfflineScanUsesCatalogueIdentityAndKeepsItOnRescan(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	offline := lib.database.Task.Create().SetType("offline").SaveX(ctx)
	payload.OfflineTaskID, payload.TargetID, payload.TargetPath = offline.ID, "download-folder", "/Movies/release-folder"
	payload.Code, payload.JavDBID = "LUXU-1899", "catalogue-id"
	entries := []pan.File{
		{ID: "prefixed", ParentID: payload.TargetID, Name: "999LUXU-1899.mp4", Size: 1 << 30, SHA1: "first-video"},
		{ID: "unnamed", ParentID: payload.TargetID, Name: "video.mp4", Size: 512 << 20, SHA1: "second-video"},
		{ID: "poster", ParentID: payload.TargetID, Name: "poster.jpg"},
	}
	identified, err := identifyScanVideosForTest(ctx, lib, payload, entries)
	if err != nil || len(identified) != 2 {
		t.Fatalf("downloaded videos = %#v, %v", identified, err)
	}
	videos := make([]scan.Video, 0, len(identified))
	for _, video := range identified {
		if video.Code != payload.Code {
			t.Fatalf("download used its filename instead of the known catalogue: %#v", video)
		}
		videos = append(videos, video)
	}
	if err := indexScanPage(ctx, lib, queued.ID, "download", payload.TargetPath, videos, &payload); err != nil {
		t.Fatal(err)
	}
	record := lib.database.Movie.Query().OnlyX(ctx)
	if record.Code != payload.Code || domain.ValueOrZero(record.JavdbID) != payload.JavDBID {
		t.Fatalf("download identity was not persisted: %#v", record)
	}
	if err := reconcileScan(ctx, lib, queued.ID, "download", &payload, nil); err != nil {
		t.Fatal(err)
	}
	metadata := lib.database.Task.Query().Where(task.TypeEQ("scrape")).OnlyX(ctx)
	input, err := tasks.DecodePayload[scrapePkg.MetadataPayload](metadata.Payload)
	if err != nil || input.MovieID != record.ID || input.JavDBID != payload.JavDBID {
		t.Fatalf("metadata job lost the known JavDB ID: %#v, %v", input, err)
	}
	full := scan.Payload{Source: payload.Source}
	restored, err := identifyScanVideosForTest(ctx, lib, full, entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, video := range restored {
		if video.Code != record.Code {
			t.Fatalf("rescan reinterpreted an unchanged filename: %#v", video)
		}
		if err := indexScanPage(ctx, lib, queued.ID, "full", payload.TargetPath, []scan.Video{video}, &full); err != nil {
			t.Fatal(err)
		}
	}
	if current := lib.database.Movie.Query().OnlyX(ctx); current.ID != record.ID {
		t.Fatalf("rescan replaced the downloaded movie: %#v", current)
	}
	if count := lib.database.File.Query().Where(file.MovieIDEQ(record.ID)).CountX(ctx); count != 2 {
		t.Fatalf("rescan lost downloaded videos: %d", count)
	}
}

func TestRescanRestoresOnlyUnchangedFilesFromTheSameAccount(t *testing.T) {
	lib, _, payload := libraryFixture(t)
	ctx := t.Context()
	known := lib.database.Movie.Create().SetCode("LUXU-1899").SetJavdbID("catalogue-id").SaveX(ctx)
	lib.database.File.Create().SetFileID("video").SetName("999LUXU-1899.mp4").SetSize(1 << 30).SetSha1("original").
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(known).SaveX(ctx)
	for _, scenario := range []struct {
		name    string
		file    pan.File
		account string
		want    string
	}{
		{name: "unchanged", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1 << 30, SHA1: "original"}, want: "LUXU-1899"},
		{name: "moved", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1 << 30, SHA1: "original", ParentID: "other-folder"}, want: "LUXU-1899"},
		{name: "renamed", file: pan.File{Name: "ABP-002.mp4", Size: 1 << 30, SHA1: "original"}, want: "ABP-002"},
		{name: "renamed without number", file: pan.File{Name: "video.mp4", Size: 1 << 30, SHA1: "original"}},
		{name: "replaced", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1 << 30, SHA1: "replacement"}, want: "999LUXU-1899"},
		{name: "different size", file: pan.File{Name: "999LUXU-1899.mp4", Size: 512 << 20, SHA1: "original"}, want: "999LUXU-1899"},
		{name: "other account", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1 << 30, SHA1: "original"}, account: "other", want: "999LUXU-1899"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := payload
			if scenario.account != "" {
				input.Source.AccountID = scenario.account
			}
			scenario.file.ID = "video"
			videos, err := identifyScanVideosForTest(ctx, lib, input, []pan.File{scenario.file})
			if err != nil || videos["video"].Code != scenario.want {
				t.Fatalf("restored video = %#v, want code %q, error = %v", videos["video"], scenario.want, err)
			}
		})
	}
}

func TestDownloadedMovieBindingPreservesKnownIdentityAndRejectsConflicts(t *testing.T) {
	for _, scenario := range []struct {
		name, code, javdbID string
		conflict            bool
	}{
		{name: "pending catalogue", code: "LUXU-1899"},
		{name: "known catalogue", code: "LUXU-1899", javdbID: "catalogue-id"},
		{name: "known ID with older spelling", code: "OLD-001", javdbID: "catalogue-id"},
		{name: "conflicting identity", code: "LUXU-1899", javdbID: "other-catalogue-id", conflict: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			lib, _, payload := libraryFixture(t)
			ctx := t.Context()
			create := lib.database.Movie.Create().SetCode(scenario.code).SetTitle("Existing title")
			if scenario.javdbID != "" {
				create.SetJavdbID(scenario.javdbID)
			}
			known := create.SaveX(ctx)
			payload.Code, payload.JavDBID = "LUXU-1899", "catalogue-id"
			var id int
			err := ent.WithTx(ctx, lib.database, func(tx *ent.Tx) error {
				var err error
				id, err = scan.IndexDownloadedMovie(ctx, tx, payload)
				return err
			})
			if (err != nil) != scenario.conflict {
				t.Fatalf("binding error = %v, conflict = %v", err, scenario.conflict)
			}
			record := lib.database.Movie.Query().OnlyX(ctx)
			if record.ID != known.ID || record.Title != known.Title {
				t.Fatalf("binding replaced existing metadata: %#v", record)
			}
			if scenario.conflict {
				if domain.ValueOrZero(record.JavdbID) != scenario.javdbID {
					t.Fatal("binding overwrote another catalogue identity")
				}
			} else if id != record.ID || domain.ValueOrZero(record.JavdbID) != payload.JavDBID {
				t.Fatalf("binding did not reuse the existing movie: %#v", record)
			}
		})
	}
}

func TestScanReconcilesOnlyCompletedRootAndKeepsOtherSources(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	old := []scan.Video{fixtureVideo("101", "ABP-001.mp4"), fixtureVideo("102", "ABP-002.mp4"), fixtureVideo("103", "ABP-003.mp4")}
	if err := indexScanPage(ctx, lib, queued.ID, "interrupted-attempt", "/Movies", old, &payload); err != nil {
		t.Fatal(err)
	}
	shared, err := lib.database.Movie.Query().Where(movie.CodeEQ("ABP-002")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct{ id, account, root string }{{"201", "100", "20"}, {"301", "200", "10"}} {
		if err := lib.database.File.Create().SetFileID(source.id).SetName("ABP-002.mp4").SetSize(1 << 30).
			SetAccountID(source.account).SetRootID(source.root).SetScanID("other").SetMovieID(shared.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := indexScanPage(ctx, lib, queued.ID, "restarted-attempt", "/Movies", old[:1], &payload); err != nil {
		t.Fatal(err)
	}
	if count, err := lib.database.File.Query().Count(ctx); err != nil || count != 5 {
		t.Fatalf("an incomplete scan pruned files: count = %d, error = %v", count, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := reconcileScan(canceled, lib, queued.ID, "restarted-attempt", &payload, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconciliation = %v", err)
	}
	if count, err := lib.database.File.Query().Count(ctx); err != nil || count != 5 {
		t.Fatalf("canceled scan pruned files: count = %d, error = %v", count, err)
	}
	if err := reconcileScan(ctx, lib, queued.ID, "restarted-attempt", &payload, nil); err != nil {
		t.Fatal(err)
	}
	if payload.Scan.RemovedFiles != 2 || payload.Scan.RemovedMovies != 1 {
		t.Fatalf("reconciliation = %#v", payload.Scan)
	}
	if count, err := lib.database.File.Query().Count(ctx); err != nil || count != 3 {
		t.Fatalf("remaining files = %d, error = %v", count, err)
	}
	if exists, err := lib.database.Movie.Query().Where(movie.IDEQ(shared.ID)).Exist(ctx); err != nil || !exists {
		t.Fatalf("movie in another root was removed: exists = %t, error = %v", exists, err)
	}
}

func TestScanPageRollsBackFilesWhenProgressCannotBeSaved(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := lib.database.Task.DeleteOneID(queued.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := indexScanPage(ctx, lib, queued.ID, "attempt", "/Movies", []scan.Video{fixtureVideo("101", "ABP-001.mp4")}, &payload); err == nil {
		t.Fatal("expected a missing task error")
	}
	if count, err := lib.database.Movie.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("partial movie write = %d, error = %v", count, err)
	}
	if count, err := lib.database.File.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("partial file write = %d, error = %v", count, err)
	}
}

func TestTaskRecoveryLeavesOfflineJobsAloneAndAllowsFailedScanRetry(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	duplicate, err := lib.EnqueueScan(ctx, payload.Source)
	if err != nil || duplicate.ID != queued.ID {
		t.Fatalf("duplicate scan = %#v, error = %v", duplicate, err)
	}
	offline, err := lib.database.Task.Create().SetType(tasks.KindOffline.String()).SetStatus(task.StatusRunning).SetProgress(40).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	job, err := lib.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan})
	if err != nil || job == nil || job.ID != queued.ID {
		t.Fatalf("claimed job = %#v, error = %v", job, err)
	}
	if err := lib.tasks.Queue().Recover(ctx, []tasks.Kind{tasks.KindScan}); err != nil {
		t.Fatal(err)
	}
	scanTask, err := lib.database.Task.Get(ctx, queued.ID)
	if err != nil || scanTask.Status != task.StatusQueued {
		t.Fatalf("recovered scan = %#v, error = %v", scanTask, err)
	}
	download, err := lib.database.Task.Get(ctx, offline.ID)
	if err != nil || download.Status != task.StatusRunning || download.Progress != 40 {
		t.Fatalf("offline task changed during recovery: %#v, error = %v", download, err)
	}
	if err := lib.tasks.Queue().Finish(ctx, queued.ID, errors.New("fixture failure")); err != nil {
		t.Fatal(err)
	}
	retry, err := lib.EnqueueScan(ctx, payload.Source)
	if err != nil || retry.ID == queued.ID || retry.Status != string(task.StatusQueued) {
		t.Fatalf("retry = %#v, error = %v", retry, err)
	}
}

func TestTargetedScanDoesNotPruneSiblingDirectories(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	for _, item := range []struct{ id, code, directory string }{
		{"101", "ABP-001.mp4", "/Movies/target"},
		{"102", "ABP-002.mp4", "/Movies/target-old"},
		{"103", "ABP-003.mp4", "/Movies/other"},
		{"104", "ABP-004.mp4", "/Movies/TARGET"},
	} {
		if err := indexScanPage(ctx, lib, queued.ID, "old", item.directory, []scan.Video{fixtureVideo(item.id, item.code)}, &payload); err != nil {
			t.Fatal(err)
		}
	}
	payload.TargetID, payload.TargetPath = "target-folder", "/Movies/target"
	if err := reconcileScan(ctx, lib, queued.ID, "new", &payload, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"102", "103", "104"} {
		if exists, err := lib.database.File.Query().Where(file.FileIDEQ(id)).Exist(ctx); err != nil || !exists {
			t.Fatalf("sibling file %s removed: %v", id, err)
		}
	}
	if exists, err := lib.database.File.Query().Where(file.FileIDEQ("101")).Exist(ctx); err != nil || exists {
		t.Fatalf("missing target file was retained: %v", err)
	}
}

func TestScanProgressDoesNotInvalidateUnchangedLibrary(t *testing.T) {
	lib, parent, payload := libraryFixture(t)
	video := []scan.Video{fixtureVideo("101", "ABP-001.mp4")}
	if err := indexScanPage(t.Context(), lib, parent.ID, "first", "/Movies", video, &payload); err != nil {
		t.Fatal(err)
	}
	revision := lib.tasks.Revisions()
	if err := indexScanPage(t.Context(), lib, parent.ID, "second", "/Movies", video, &payload); err != nil {
		t.Fatal(err)
	}
	if err := scan.ReportScan(t.Context(), lib.database.Task, parent.ID, payload, lib.tasks); err != nil {
		t.Fatal(err)
	}
	if got := lib.tasks.Revisions(); got != revision || got.Library != 1 {
		t.Fatalf("progress invalidated library: before=%+v after=%+v", revision, got)
	}
}

type testMediaNotifier struct {
	mu       sync.Mutex
	notified []string
}

func (n *testMediaNotifier) NotifyUpdated(path string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notified = append(n.notified, path)
}

func TestScanReconcile_CleansUpEmbyDirectoryAndNotifiesEmbyOnMovieDeletion(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	tempDir := t.TempDir()
	embyDir := filepath.Join(tempDir, "emby")

	// 1. Initial scan: index ABP-001.mp4
	old := []scan.Video{fixtureVideo("101", "ABP-001.mp4")}
	if err := indexScanPage(ctx, lib, queued.ID, "attempt-1", "/Movies", old, &payload); err != nil {
		t.Fatal(err)
	}

	// 2. Simulate exported Emby files on local disk
	movieDir := filepath.Join(embyDir, "ABP", "ABP-001")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "ABP-001.strm"), []byte("strm content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "ABP-001.nfo"), []byte("nfo content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "poster.jpg"), []byte("poster data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify local files exist before deletion
	if _, err := os.Stat(filepath.Join(movieDir, "ABP-001.strm")); err != nil {
		t.Fatalf("strm file not created: %v", err)
	}

	// 3. User deletes ABP-001 in 115. Rescan discovers a different video (SSIS-999.mp4), omitting ABP-001
	newVideos := []scan.Video{fixtureVideo("202", "SSIS-999.mp4")}
	if err := indexScanPage(ctx, lib, queued.ID, "attempt-2", "/Movies", newVideos, &payload); err != nil {
		t.Fatal(err)
	}

	notifier := &testMediaNotifier{}
	if err := reconcileScan(ctx, lib, queued.ID, "attempt-2", &payload, nil, embyDir, "http://127.0.0.1:8080", "tok", notifier); err != nil {
		t.Fatalf("reconcileScan failed: %v", err)
	}

	// 4. Verify scan stats
	if payload.Scan.RemovedFiles != 1 || payload.Scan.RemovedMovies != 1 {
		t.Fatalf("expected 1 removed file and 1 removed movie, got %+v", payload.Scan)
	}

	// 5. Verify local movie directory and all sidecars were deleted
	if _, err := os.Stat(movieDir); !os.IsNotExist(err) {
		t.Fatalf("expected movieDir %s to be deleted, got err: %v", movieDir, err)
	}

	// 6. Verify empty prefix folder was also deleted
	prefixDir := filepath.Join(embyDir, "ABP")
	if _, err := os.Stat(prefixDir); !os.IsNotExist(err) {
		t.Fatalf("expected empty prefixDir %s to be deleted, got err: %v", prefixDir, err)
	}

	// 7. Verify notifier was notified of the deletion
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.notified) != 1 || notifier.notified[0] != movieDir {
		t.Fatalf("expected notifier to receive %s, got: %+v", movieDir, notifier.notified)
	}

	// 8. Verify database movie record is removed
	exists, err := lib.database.Movie.Query().Where(movie.CodeEQ("ABP-001")).Exist(ctx)
	if err != nil || exists {
		t.Fatalf("expected ABP-001 to be removed from DB: exists=%t, err=%v", exists, err)
	}
}
