package service

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func libraryFixture(t *testing.T) (*LibraryService, TaskInfo, scanPayload) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	source := LibrarySource{AccountID: "100", Directory: PanLibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}}
	if err := saveSetting(t.Context(), store.Client, panDirectorySetting, panLibraryDirectory{
		AccountID: source.AccountID, PanLibraryDirectory: source.Directory,
	}); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskService(store.Client)
	queued, err := tasks.enqueueScan(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	images, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewLibraryService(store.Client, nil, tasks, images), queued, scanPayload{Source: source, Scan: ScanProgress{Stage: "scanning"}}
}

func fixtureVideo(id, name string) scanVideo {
	return identifyVideo(pan.File{ID: id, ParentID: "10", Name: name, Size: 1024})
}

func TestScanCombinesPartsPreservesMetadataAndRetainsUnmatchedFiles(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	metadata, err := library.database.Movie.Create().SetCode("ABP-001").SetTitle("Existing title").
		SetJavdbID("fixture").SetScrapeStatus(movie.ScrapeStatusDone).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	videos := []scanVideo{
		fixtureVideo("101", "abp001-CD1.mp4"),
		fixtureVideo("102", "ABP-001-CD2.mkv"),
		fixtureVideo("103", "recording.mp4"),
	}
	for _, marker := range []string{"first", "second"} {
		if err := library.indexScanPage(ctx, queued.ID, marker, "/Movies", videos, &payload); err != nil {
			t.Fatal(err)
		}
	}
	page, err := library.Movies(ctx, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.FileCount != 3 || page.UnmatchedFiles != 1 || len(page.Movies) != 1 {
		t.Fatalf("library = %#v", page)
	}
	item := page.Movies[0]
	if item.ID != metadata.ID || item.Title != metadata.Title || item.ScrapeStatus != movie.ScrapeStatusDone || item.FileCount != 2 || item.Size != 2048 {
		t.Fatalf("scanned movie = %#v", item)
	}
	// A renamed video that no longer identifies a code must not keep a stale
	// association, while the other part still keeps the movie in the library.
	if err := library.indexScanPage(ctx, queued.ID, "third", "/Movies", []scanVideo{fixtureVideo("102", "recording-two.mkv")}, &payload); err != nil {
		t.Fatal(err)
	}
	renamed, err := library.database.File.Query().Where(file.FileIDEQ("102")).Only(ctx)
	if err != nil || renamed.MovieID != nil {
		t.Fatalf("renamed video = %#v, error = %v", renamed, err)
	}
	remaining, err := library.database.Movie.Get(ctx, metadata.ID)
	if err != nil || remaining.JavdbID == nil || *remaining.JavdbID != "fixture" {
		t.Fatalf("remaining metadata = %#v, error = %v", remaining, err)
	}
}

func TestScanKeepsLetterSerialsDistinctAndDoesNotExtractPartialNumbers(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := library.indexScanPage(ctx, queued.ID, "fixture", "/Movies", []scanVideo{
		fixtureVideo("letter", "KNB-M014.mp4"),
		fixtureVideo("short", "M-014.mp4"),
		fixtureVideo("unidentified", "UNKNOWN-KNB-M014.mp4"),
	}, &payload); err != nil {
		t.Fatal(err)
	}
	page, err := library.Movies(ctx, 1, 24)
	if err != nil || page.Total != 2 || page.UnmatchedFiles != 1 {
		t.Fatalf("letter serials were lost or merged: %#v, %v", page, err)
	}
	ids := make(map[string]int)
	for _, item := range page.Movies {
		ids[item.Code] = item.ID
	}
	if ids["KNB-M014"] == 0 || ids["M-014"] == 0 || ids["KNB-M014"] == ids["M-014"] {
		t.Fatalf("incorrect catalogue identities: %#v", ids)
	}
	unknown := library.database.File.Query().Where(file.FileIDEQ("unidentified")).OnlyX(ctx)
	if unknown.MovieID != nil {
		t.Fatal("unrecognized filename was associated with a partial catalogue number")
	}
}

func TestScanCombinesCatalogueAliasesAndReplacesFailedLegacyIndex(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	legacy := library.database.Movie.Create().SetCode("259LUXU-1899").
		SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
	library.database.File.Create().SetFileID("prefixed").SetName("259LUXU-1899.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(legacy).SaveX(ctx)
	known := library.database.Movie.Create().SetCode("LUXU-1899").SetTitle("Catalogue title").
		SetJavdbID("catalogue-id").SetScrapeStatus(movie.ScrapeStatusDone).SaveX(ctx)
	videos := []scanVideo{
		fixtureVideo("prefixed", "259LUXU-1899.mp4"),
		fixtureVideo("catalogue", "LUXU-1899-CD2.mkv"),
	}
	for _, marker := range []string{"first", "rescan"} {
		if err := library.indexScanPage(ctx, queued.ID, marker, "/Movies", videos, &payload); err != nil {
			t.Fatal(err)
		}
	}
	page, err := library.Movies(ctx, 1, 24)
	if err != nil || page.Total != 1 || len(page.Movies) != 1 {
		t.Fatalf("catalogue aliases created duplicate movies: %#v, %v", page, err)
	}
	item := page.Movies[0]
	if item.ID != known.ID || item.Code != "LUXU-1899" || item.FileCount != 2 ||
		item.Title != known.Title || item.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("alias scan lost existing metadata or file associations: %#v", item)
	}
	if _, err := library.database.Movie.Get(ctx, legacy.ID); !ent.IsNotFound(err) {
		t.Fatalf("unreferenced legacy alias was not removed: %v", err)
	}
}

func TestMetadataCanonicalizesLegacyAliasBeforeRescan(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	legacy := library.database.Movie.Create().SetCode("259LUXU-1899").SaveX(ctx)
	library.database.File.Create().SetFileID("prefixed").SetName("259LUXU-1899.mp4").SetSize(1024).
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(legacy).SaveX(ctx)
	doc := nfo.Movie{Code: "LUXU-1899", Title: "Catalogue title",
		IDs: []nfo.UniqueID{{Type: "javdb", Default: true, Value: "catalogue-id"}},
	}
	if err := ent.WithTx(ctx, library.database, func(tx *ent.Tx) error {
		return saveMovieMetadata(ctx, tx, legacy.ID, doc)
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.indexScanPage(ctx, queued.ID, "rescan", "/Movies",
		[]scanVideo{fixtureVideo("prefixed", "259LUXU-1899.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	record := library.database.Movie.Query().OnlyX(ctx)
	if record.ID != legacy.ID || record.Code != doc.Code || record.Title != doc.Title || valueOrZero(record.JavdbID) != doc.JavDBID() {
		t.Fatalf("metadata normalization changed the movie identity on rescan: %#v", record)
	}
}

func TestOfflineScanUsesCatalogueIdentityAndKeepsItOnRescan(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	offline := library.database.Task.Create().SetType("offline").SaveX(ctx)
	payload.OfflineTaskID, payload.TargetID, payload.TargetPath = offline.ID, "download-folder", "/Movies/release-folder"
	payload.Code, payload.JavDBID = "LUXU-1899", "catalogue-id"
	entries := []pan.File{
		{ID: "prefixed", ParentID: payload.TargetID, Name: "999LUXU-1899.mp4", Size: 1024, SHA1: "first-video"},
		{ID: "unnamed", ParentID: payload.TargetID, Name: "video.mp4", Size: 512, SHA1: "second-video"},
		{ID: "poster", ParentID: payload.TargetID, Name: "poster.jpg"},
	}
	identified, err := library.identifyScanVideos(ctx, payload, entries)
	if err != nil || len(identified) != 2 {
		t.Fatalf("downloaded videos = %#v, %v", identified, err)
	}
	videos := make([]scanVideo, 0, len(identified))
	for _, video := range identified {
		if video.Code != payload.Code {
			t.Fatalf("download used its filename instead of the known catalogue: %#v", video)
		}
		videos = append(videos, video)
	}
	if err := library.indexScanPage(ctx, queued.ID, "download", payload.TargetPath, videos, &payload); err != nil {
		t.Fatal(err)
	}
	record := library.database.Movie.Query().OnlyX(ctx)
	if record.Code != payload.Code || valueOrZero(record.JavdbID) != payload.JavDBID {
		t.Fatalf("download identity was not persisted: %#v", record)
	}
	if err := library.reconcileScan(ctx, queued.ID, "download", &payload, nil); err != nil {
		t.Fatal(err)
	}
	metadata := library.database.Task.Query().Where(task.TypeEQ("scrape")).OnlyX(ctx)
	input, err := decodeTaskPayload[metadataPayload](metadata.Payload)
	if err != nil || input.MovieID != record.ID || input.JavDBID != payload.JavDBID {
		t.Fatalf("metadata job lost the known JavDB ID: %#v, %v", input, err)
	}
	full := scanPayload{Source: payload.Source}
	restored, err := library.identifyScanVideos(ctx, full, entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, video := range restored {
		if video.Code != record.Code {
			t.Fatalf("rescan reinterpreted an unchanged filename: %#v", video)
		}
		if err := library.indexScanPage(ctx, queued.ID, "full", payload.TargetPath, []scanVideo{video}, &full); err != nil {
			t.Fatal(err)
		}
	}
	if current := library.database.Movie.Query().OnlyX(ctx); current.ID != record.ID {
		t.Fatalf("rescan replaced the downloaded movie: %#v", current)
	}
	if count := library.database.File.Query().Where(file.MovieIDEQ(record.ID)).CountX(ctx); count != 2 {
		t.Fatalf("rescan lost downloaded videos: %d", count)
	}
}

func TestRescanRestoresOnlyUnchangedFilesFromTheSameAccount(t *testing.T) {
	library, _, payload := libraryFixture(t)
	ctx := t.Context()
	known := library.database.Movie.Create().SetCode("LUXU-1899").SetJavdbID("catalogue-id").SaveX(ctx)
	library.database.File.Create().SetFileID("video").SetName("999LUXU-1899.mp4").SetSize(1024).SetSha1("original").
		SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(known).SaveX(ctx)
	for _, scenario := range []struct {
		name    string
		file    pan.File
		account string
		want    string
	}{
		{name: "unchanged", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1024, SHA1: "original"}, want: "LUXU-1899"},
		{name: "moved", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1024, SHA1: "original", ParentID: "other-folder"}, want: "LUXU-1899"},
		{name: "renamed", file: pan.File{Name: "ABP-002.mp4", Size: 1024, SHA1: "original"}, want: "ABP-002"},
		{name: "renamed without number", file: pan.File{Name: "video.mp4", Size: 1024, SHA1: "original"}},
		{name: "replaced", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1024, SHA1: "replacement"}, want: "999LUXU-1899"},
		{name: "different size", file: pan.File{Name: "999LUXU-1899.mp4", Size: 512, SHA1: "original"}, want: "999LUXU-1899"},
		{name: "other account", file: pan.File{Name: "999LUXU-1899.mp4", Size: 1024, SHA1: "original"}, account: "other", want: "999LUXU-1899"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input := payload
			if scenario.account != "" {
				input.Source.AccountID = scenario.account
			}
			scenario.file.ID = "video"
			videos, err := library.identifyScanVideos(ctx, input, []pan.File{scenario.file})
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
			library, _, payload := libraryFixture(t)
			ctx := t.Context()
			create := library.database.Movie.Create().SetCode(scenario.code).SetTitle("Existing title")
			if scenario.javdbID != "" {
				create.SetJavdbID(scenario.javdbID)
			}
			known := create.SaveX(ctx)
			payload.Code, payload.JavDBID = "LUXU-1899", "catalogue-id"
			var id int
			err := ent.WithTx(ctx, library.database, func(tx *ent.Tx) error {
				var err error
				id, err = indexDownloadedMovie(ctx, tx, payload)
				return err
			})
			if (err != nil) != scenario.conflict {
				t.Fatalf("binding error = %v, conflict = %v", err, scenario.conflict)
			}
			record := library.database.Movie.Query().OnlyX(ctx)
			if record.ID != known.ID || record.Title != known.Title {
				t.Fatalf("binding replaced existing metadata: %#v", record)
			}
			if scenario.conflict {
				if valueOrZero(record.JavdbID) != scenario.javdbID {
					t.Fatal("binding overwrote another catalogue identity")
				}
			} else if id != record.ID || valueOrZero(record.JavdbID) != payload.JavDBID {
				t.Fatalf("binding did not reuse the existing movie: %#v", record)
			}
		})
	}
}

func TestScanReconcilesOnlyCompletedRootAndKeepsOtherSources(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	old := []scanVideo{fixtureVideo("101", "ABP-001.mp4"), fixtureVideo("102", "ABP-002.mp4"), fixtureVideo("103", "ABP-003.mp4")}
	if err := library.indexScanPage(ctx, queued.ID, "interrupted-attempt", "/Movies", old, &payload); err != nil {
		t.Fatal(err)
	}
	shared, err := library.database.Movie.Query().Where(movie.CodeEQ("ABP-002")).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct{ id, account, root string }{{"201", "100", "20"}, {"301", "200", "10"}} {
		if err := library.database.File.Create().SetFileID(source.id).SetName("ABP-002.mp4").SetSize(1024).
			SetAccountID(source.account).SetRootID(source.root).SetScanID("other").SetMovieID(shared.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := library.indexScanPage(ctx, queued.ID, "restarted-attempt", "/Movies", old[:1], &payload); err != nil {
		t.Fatal(err)
	}
	if count, err := library.database.File.Query().Count(ctx); err != nil || count != 5 {
		t.Fatalf("an incomplete scan pruned files: count = %d, error = %v", count, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := library.reconcileScan(canceled, queued.ID, "restarted-attempt", &payload, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconciliation = %v", err)
	}
	if count, err := library.database.File.Query().Count(ctx); err != nil || count != 5 {
		t.Fatalf("canceled scan pruned files: count = %d, error = %v", count, err)
	}
	if err := library.reconcileScan(ctx, queued.ID, "restarted-attempt", &payload, nil); err != nil {
		t.Fatal(err)
	}
	if payload.Scan.RemovedFiles != 2 || payload.Scan.RemovedMovies != 1 {
		t.Fatalf("reconciliation = %#v", payload.Scan)
	}
	if count, err := library.database.File.Query().Count(ctx); err != nil || count != 3 {
		t.Fatalf("remaining files = %d, error = %v", count, err)
	}
	if exists, err := library.database.Movie.Query().Where(movie.IDEQ(shared.ID)).Exist(ctx); err != nil || !exists {
		t.Fatalf("movie in another root was removed: exists = %t, error = %v", exists, err)
	}
}

func TestScanPageRollsBackFilesWhenProgressCannotBeSaved(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := library.database.Task.DeleteOneID(queued.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := library.indexScanPage(ctx, queued.ID, "attempt", "/Movies", []scanVideo{fixtureVideo("101", "ABP-001.mp4")}, &payload); err == nil {
		t.Fatal("expected a missing task error")
	}
	if count, err := library.database.Movie.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("partial movie write = %d, error = %v", count, err)
	}
	if count, err := library.database.File.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("partial file write = %d, error = %v", count, err)
	}
}

func TestTaskRecoveryLeavesOfflineJobsAloneAndAllowsFailedScanRetry(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	duplicate, err := library.tasks.enqueueScan(ctx, payload.Source)
	if err != nil || duplicate.ID != queued.ID {
		t.Fatalf("duplicate scan = %#v, error = %v", duplicate, err)
	}
	offline, err := library.database.Task.Create().SetType("offline").SetStatus(task.StatusRunning).SetProgress(40).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	job, err := library.tasks.Claim(ctx, []string{"scan"})
	if err != nil || job == nil || job.ID != queued.ID {
		t.Fatalf("claimed job = %#v, error = %v", job, err)
	}
	if err := library.tasks.Recover(ctx, []string{"scan"}); err != nil {
		t.Fatal(err)
	}
	scan, err := library.database.Task.Get(ctx, queued.ID)
	if err != nil || scan.Status != task.StatusQueued {
		t.Fatalf("recovered scan = %#v, error = %v", scan, err)
	}
	download, err := library.database.Task.Get(ctx, offline.ID)
	if err != nil || download.Status != task.StatusRunning || download.Progress != 40 {
		t.Fatalf("offline task changed during recovery: %#v, error = %v", download, err)
	}
	if err := library.tasks.Finish(ctx, queued.ID, errors.New("fixture failure")); err != nil {
		t.Fatal(err)
	}
	retry, err := library.tasks.enqueueScan(ctx, payload.Source)
	if err != nil || retry.ID == queued.ID || retry.Status != task.StatusQueued {
		t.Fatalf("retry = %#v, error = %v", retry, err)
	}
}

func TestTargetedScanDoesNotPruneSiblingDirectories(t *testing.T) {
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	for _, item := range []struct{ id, code, directory string }{
		{"101", "ABP-001.mp4", "/Movies/target"},
		{"102", "ABP-002.mp4", "/Movies/target-old"},
		{"103", "ABP-003.mp4", "/Movies/other"},
		{"104", "ABP-004.mp4", "/Movies/TARGET"},
	} {
		if err := library.indexScanPage(ctx, queued.ID, "old", item.directory, []scanVideo{fixtureVideo(item.id, item.code)}, &payload); err != nil {
			t.Fatal(err)
		}
	}
	payload.TargetID, payload.TargetPath = "target-folder", "/Movies/target"
	if err := library.reconcileScan(ctx, queued.ID, "new", &payload, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"102", "103", "104"} {
		if exists, err := library.database.File.Query().Where(file.FileIDEQ(id)).Exist(ctx); err != nil || !exists {
			t.Fatalf("sibling file %s removed: %v", id, err)
		}
	}
	if exists, err := library.database.File.Query().Where(file.FileIDEQ("101")).Exist(ctx); err != nil || exists {
		t.Fatalf("missing target file was retained: %v", err)
	}
}
