package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanResumesCanceledDirectoryAndPersistsAllChunks(t *testing.T) {
	lib, client := panConcurrencyFixture(t)
	ctx := t.Context()
	source := *lib.drive.Source()
	entries := make([]pan.File, 205)
	for i := range entries {
		entries[i] = pan.File{ID: fmt.Sprintf("video-%d", i), ParentID: source.Directory.ID,
			Name: fmt.Sprintf("ABP-%03d.mp4", i+1), Size: domain.MinVideoSize}
	}
	interrupted, cancel := context.WithCancel(ctx)
	defer cancel()
	stopOnSecondPage := true
	client.list = func(_ context.Context, _, directoryID string, offset, _ int) (pan.FilePage, error) {
		if directoryID != source.Directory.ID {
			t.Fatalf("unexpected directory %q", directoryID)
		}
		if stopOnSecondPage && offset == 100 {
			stopOnSecondPage = false
			cancel()
			return pan.FilePage{}, context.Canceled
		}
		end := min(offset+100, len(entries))
		return pan.FilePage{Files: entries[offset:end], Path: []pan.Directory{{ID: source.Directory.ID}},
			Total: len(entries), HasMore: end < len(entries)}, nil
	}
	queued := lib.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	if err := lib.Scan(interrupted, tasks.Job{ID: queued.ID, Payload: queued.Payload}); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan cancellation: %v", err)
	}
	if count := lib.database.File.Query().CountX(ctx); count != 0 {
		t.Fatalf("partial directory committed %d files", count)
	}
	queued = lib.database.Task.GetX(ctx, queued.ID)
	before, err := tasks.DecodePayload[scan.Payload](queued.Payload)
	if err != nil || before.ScanID == "" || before.Checkpoint == "" {
		t.Fatalf("restart context missing: %+v %v", before, err)
	}
	if err := lib.Scan(ctx, tasks.Job{ID: queued.ID, Payload: queued.Payload}); err != nil {
		t.Fatal(err)
	}
	after, err := tasks.DecodePayload[scan.Payload](lib.database.Task.GetX(ctx, queued.ID).Payload)
	if err != nil {
		t.Fatal(err)
	}
	if after.ScanID != before.ScanID || after.Checkpoint != "" || after.Scan.Stage != "done" ||
		after.Scan.FilesScanned != 205 || after.Scan.MatchedFiles != 205 || after.Scan.Movies != 205 || after.Scan.DirectoriesScanned != 1 {
		t.Fatalf("unexpected resumed progress: %+v", after)
	}
	if count := lib.database.File.Query().Where(file.ScanIDEQ(before.ScanID)).CountX(ctx); count != 205 {
		t.Fatalf("expected 205 files across all commit chunks, got %d", count)
	}
}

func TestScanProgressKeepsRestartContextAndScanKind(t *testing.T) {
	for _, targeted := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "targeted"}[targeted], func(t *testing.T) {
			lib, queued, payload := libraryFixture(t)
			payload.Scan.FilesScanned = 100
			if targeted {
				payload.TargetID, payload.TargetPath, payload.TargetFile = "video", "/Movies/video.mp4", true
				payload.OfflineTaskID, payload.Code, payload.JavDBID = 123, "ABP-001", "catalogue-id"
			}
			if err := scan.SaveScanProgress(t.Context(), lib.database.Task, queued.ID, payload); err != nil {
				t.Fatal(err)
			}
			record := lib.database.Task.GetX(t.Context(), queued.ID)
			restored, err := tasks.DecodePayload[scan.Payload](record.Payload)
			if err != nil || restored != payload {
				t.Fatalf("restart context changed: %+v err=%v", restored, err)
			}
			next, err := lib.EnqueueScan(t.Context(), payload.Source)
			if err != nil || (next.ID == queued.ID) == targeted {
				t.Fatalf("full-scan deduplication confused targeted=%t: %+v err=%v", targeted, next, err)
			}
		})
	}
}

func TestMixedScanPageRollsBackBothChangedFilesAndUnchangedMarkers(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	videos := []scan.Video{fixtureVideo("101", "ABP-001.mp4"), fixtureVideo("102", "ABP-002.mp4")}
	if err := indexScanPage(t.Context(), lib, queued.ID, "previous", "/Movies", videos, &payload); err != nil {
		t.Fatal(err)
	}
	lib.database.Task.DeleteOneID(queued.ID).ExecX(t.Context())
	videos[1] = fixtureVideo("102", "ABP-003.mp4")
	payload.ScanID = "failed"
	if err := scan.ProcessScanPage(t.Context(), lib.database, queued.ID, "/Movies", videos, &payload,
		func(videos []scan.Video) []scan.Video { return videos }, lib.tasks); err == nil {
		t.Fatal("scan page without a progress record unexpectedly committed")
	}
	for _, id := range []string{"101", "102"} {
		if got := lib.database.File.Query().Where(file.FileIDEQ(id)).OnlyX(t.Context()); got.ScanID != "previous" {
			t.Fatalf("file %s escaped rollback: %+v", id, got)
		}
	}
	if got := lib.database.File.Query().Where(file.FileIDEQ("102")).OnlyX(t.Context()); got.Name != "ABP-002.mp4" {
		t.Fatal("renamed file escaped rollback")
	}
	if lib.database.Movie.Query().CountX(t.Context()) != 2 {
		t.Fatal("new movie escaped rollback")
	}
}

func TestScanResolvesAndIndexesCurrentIdentityWithOneFileRead(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	film := lib.database.Movie.Create().SetCode("OLD-001").SetJavdbID("catalogue-id").SaveX(ctx)
	lib.database.File.Create().SetFileID("video").SetName("video.mp4").SetParentID("10").SetPath("/Movies/video.mp4").
		SetSize(1 << 30).SetSha1("original").SetAccountID(payload.Source.AccountID).SetRootID(payload.Source.Directory.ID).SetMovie(film).ExecX(ctx)
	// An already prepared filename/code must not override fresher catalogue data.
	videos := []scan.Video{{File: pan.File{ID: "video", ParentID: "10", Name: "video.mp4", Size: 1 << 30, SHA1: "original"}, Code: "STALE-001"}}
	film.Update().SetCode("CURRENT-001").ExecX(ctx)
	fileReads, movieReads := 0, 0
	lib.database.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			switch query.(type) {
			case *ent.FileQuery:
				fileReads++
			case *ent.MovieQuery:
				movieReads++
			}
			return next.Query(ctx, query)
		})
	}))
	payload.ScanID = "current"
	if err := scan.ProcessScanPage(ctx, lib.database, queued.ID, "/Movies", videos, &payload,
		func(videos []scan.Video) []scan.Video { return videos }, lib.tasks); err != nil {
		t.Fatal(err)
	}
	if fileReads != 1 || movieReads != 1 || videos[0].Code != "CURRENT-001" {
		t.Fatalf("scan reread or used stale identities: files=%d movies=%d videos=%#v", fileReads, movieReads, videos)
	}
	indexed := lib.database.File.Query().Where(file.FileIDEQ("video")).OnlyX(ctx)
	if indexed.MovieID == nil || *indexed.MovieID != film.ID || indexed.ScanID != "current" {
		t.Fatalf("scan changed a verified association: %#v", indexed)
	}
}

func TestCompactScanKeepsSharedDirectoryAndArtworkMatching(t *testing.T) {
	for _, scenario := range []struct {
		name, nfo, poster string
		shared            bool
		jobs              int
	}{
		{name: "generic NFO", nfo: "movie.nfo", poster: "cover.asset"},
		{name: "generic NFO with another video", nfo: "movie.nfo", poster: "cover.asset", shared: true, jobs: 1},
		{name: "exact NFO with another video", nfo: "ABP-001.nfo", poster: "cover.asset", shared: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newCompletedScanFixture(t)
			f.entries["10"][1].Name, f.entries["10"][2].Name = scenario.nfo, scenario.poster
			f.snapshot.Directories[0].NFO.Name = scenario.nfo
			f.snapshot.Directories[0].Poster.Name = scenario.poster
			// A directory ending in .nfo must not become a candidate sidecar.
			f.entries["10"] = append(f.entries["10"], pan.File{ID: "folder", Name: "other.nfo", IsDirectory: true})
			if scenario.shared {
				f.entries["10"] = append(f.entries["10"], pan.File{ID: "unmatched", Name: "recording.mp4", Size: 1 << 30})
			}
			f.movie.Update().SetMetadataSnapshot(f.snapshot).ExecX(t.Context())
			observed := make(scrape.DirectoryObservations)
			for _, entry := range f.entries["10"] {
				observed.Add("10", []pan.File{entry})
			}
			if err := indexScanPage(t.Context(), f.lib, f.queued.ID, "rescan", "/Movies", f.videos, &f.payload); err != nil {
				t.Fatal(err)
			}
			if err := reconcileScan(t.Context(), f.lib, f.queued.ID, "rescan", &f.payload, observed); err != nil {
				t.Fatal(err)
			}
			if count := f.lib.database.Task.Query().Where(task.TypeEQ("scrape")).CountX(t.Context()); count != scenario.jobs {
				t.Fatalf("metadata tasks=%d want=%d", count, scenario.jobs)
			}
		})
	}
}

func TestScanCheckpointResumePreservesPreviousScannedFiles(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "legacy"}[legacy], func(t *testing.T) {
			lib, client := panConcurrencyFixture(t)
			ctx := t.Context()
			source := *lib.drive.Source()

			rootDirID := source.Directory.ID
			entries := map[string][]pan.File{
				rootDirID: {
					{ID: "dir-1", ParentID: rootDirID, Name: "Dir1", IsDirectory: true},
					{ID: "dir-2", ParentID: rootDirID, Name: "Dir2", IsDirectory: true},
				},
				"dir-1": {
					{ID: "v1", ParentID: "dir-1", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "s1"},
				},
				"dir-2": {
					{ID: "v2", ParentID: "dir-2", Name: "ABP-002.mp4", Size: 1 << 30, SHA1: "s2"},
				},
			}
			client.list = func(_ context.Context, _, id string, offset, _ int) (pan.FilePage, error) {
				if offset != 0 {
					return pan.FilePage{}, nil
				}
				files := entries[id]
				return pan.FilePage{Files: files, Total: len(files), Path: []pan.Directory{{ID: rootDirID}}}, nil
			}

			scanID := "test-scan-uuid-123"
			payload := scan.Payload{
				ScanID: scanID,
				Source: source,
				Scan: domain.ScanProgress{
					Stage:                 "scanning",
					CurrentPath:           "/Movies/Dir1",
					DirectoriesDiscovered: 2,
					DirectoriesScanned:    1,
					FilesScanned:          1,
					VideoFiles:            1,
					MatchedFiles:          1,
					Movies:                1,
				},
			}
			v1 := scan.IdentifyVideo(pan.File{ID: "v1", ParentID: "dir-1", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "s1"})
			queued := lib.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)

			payload.ScanID = scanID

			if err := scan.ProcessScanPage(ctx, lib.database, queued.ID, "/Movies/Dir1", []scan.Video{v1}, &payload, nil, lib.tasks); err != nil {
				t.Fatal(err)
			}

			cp, err := json.Marshal([]scan.Directory{{ID: "dir-2", Path: "/Movies/Dir2"}})
			if err != nil {
				t.Fatal(err)
			}
			payload.Checkpoint = string(cp)
			if legacy {
				payload.ScanID = ""
			}
			if err := scan.SaveScanProgress(ctx, lib.database.Task, queued.ID, payload); err != nil {
				t.Fatal(err)
			}

			taskRecord := lib.database.Task.GetX(ctx, queued.ID)
			if err := lib.Scan(ctx, tasks.Job{ID: taskRecord.ID, Payload: taskRecord.Payload}); err != nil {
				t.Fatal(err)
			}

			file1, err := lib.database.File.Query().Where(file.FileIDEQ("v1")).Only(ctx)
			if err != nil {
				t.Fatalf("file v1 from dir-1 was incorrectly deleted during resume reconciliation: %v", err)
			}
			if !legacy && file1.ScanID != scanID {
				t.Fatalf("expected file1 scanID %s, got %s", scanID, file1.ScanID)
			}
			file2, err := lib.database.File.Query().Where(file.FileIDEQ("v2")).Only(ctx)
			if err != nil {
				t.Fatalf("file v2 from dir-2 was not indexed: %v", err)
			}
			if file2.ScanID != file1.ScanID {
				t.Fatalf("expected file2 scanID %s, got %s", scanID, file2.ScanID)
			}

			if !lib.database.Movie.Query().Where(movie.CodeEQ("ABP-001")).ExistX(ctx) {
				t.Fatal("ABP-001 movie was incorrectly deleted during resume reconciliation")
			}
			if !lib.database.Movie.Query().Where(movie.CodeEQ("ABP-002")).ExistX(ctx) {
				t.Fatal("ABP-002 movie was not indexed")
			}

			resumedTask := lib.database.Task.GetX(ctx, queued.ID)
			resumedPayload, err := tasks.DecodePayload[scan.Payload](resumedTask.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if resumedPayload.Scan.Stage != "done" {
				t.Fatalf("expected stage 'done', got %q", resumedPayload.Scan.Stage)
			}
			if resumedPayload.Checkpoint != "" {
				t.Fatalf("expected empty checkpoint, got %q", resumedPayload.Checkpoint)
			}
			if resumedPayload.Scan.Movies != 2 {
				t.Fatalf("expected 2 movies in scan progress, got %d", resumedPayload.Scan.Movies)
			}
		})
	}
}

func TestReconcileRollbackKeepsExport(t *testing.T) {
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	if err := indexScanPage(ctx, lib, queued.ID, "old", "/Movies", []scan.Video{fixtureVideo("101", "ABP-001.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := scrape.EmbyMovieDir(root, "ABP-001")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	exported := filepath.Join(dir, "ABP-001.strm")
	if err := os.WriteFile(exported, []byte("saved-media"), 0644); err != nil {
		t.Fatal(err)
	}
	// A missing task causes SaveScanProgress to fail after reconciliation work.
	payload.ScanID = "new"
	err := scan.ReconcileScan(ctx, lib.database, -1, &payload, nil, nil, nil, export.Config{EmbyDir: root}, nil)
	if err == nil {
		t.Fatal("expected rollback")
	}
	if count := lib.database.Movie.Query().CountX(ctx); count != 1 {
		t.Fatalf("database did not roll back: %d", count)
	}
	if _, err := os.Stat(exported); err != nil {
		t.Fatalf("export deleted despite database rollback: %v", err)
	}
}
