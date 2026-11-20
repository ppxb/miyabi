package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanRepairsReportedLetterPartsAndExportsInOrder(t *testing.T) {
	lib, client := panConcurrencyFixture(t)
	ctx, source := t.Context(), *lib.drive.Source()
	cfg := export.Config{EmbyDir: t.TempDir(), PublicURL: "http://localhost"}
	lib.scanner = scan.New(lib.drive, lib.database, lib.images, lib.tasks, export.NewManager(cfg), func(context.Context) error { return nil })
	// The real 115 sample lists B first; each part is also on a separate page.
	videos := []pan.File{
		{ID: "3537613304031610685", ParentID: "parts", Name: "onsd-340B.mp4", Size: 914101803},
		{ID: "3537613304031610684", ParentID: "parts", Name: "onsd-340A.mp4", Size: 1899803752},
	}
	for _, video := range videos {
		code := scan.IdentifyVideo(video).Code
		film := lib.database.Movie.Create().SetCode(code).SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
		lib.database.File.Create().SetFileID(video.ID).SetName(video.Name).SetSize(video.Size).
			SetParentID(video.ParentID).SetAccountID(source.AccountID).SetRootID(source.Directory.ID).
			SetPath("/Movies/onsd-340/" + video.Name).SetMovie(film).ExecX(ctx)
		if _, err := export.WriteMedia(export.Config{EmbyDir: cfg.EmbyDir, PublicURL: cfg.PublicURL, STRMToken: ""}, code, nfo.Movie{Code: code}, []pan.File{video}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	client.list = func(_ context.Context, _, id string, offset, _ int) (pan.FilePage, error) {
		if id == source.Directory.ID {
			return pan.FilePage{Files: []pan.File{{ID: "parts", Name: "onsd-340", IsDirectory: true}}, Total: 1,
				Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
		}
		if id != "parts" || offset >= len(videos) {
			return pan.FilePage{}, fmt.Errorf("unexpected directory page %s/%d", id, offset)
		}
		return pan.FilePage{Files: videos[offset : offset+1], Total: 2, HasMore: offset == 0,
			Path: []pan.Directory{{ID: source.Directory.ID}, {ID: "parts"}}}, nil
	}
	for attempt := range 2 {
		body, _ := tasks.EncodePayload(domain.ScanPayload{Source: source})
		job := lib.database.Task.Create().SetType("scan").SetPayload(body).SaveX(ctx)
		if err := lib.Scan(ctx, tasks.Job{ID: job.ID, Payload: job.Payload}); err != nil {
			t.Fatal(err)
		}
		film := lib.database.Movie.Query().WithFiles().OnlyX(ctx)
		if film.Code != "ONSD-340" || len(film.Edges.Files) != 2 || lib.database.File.Query().CountX(ctx) != 2 {
			t.Fatalf("rescan %d did not group the original files: %+v", attempt, film)
		}
		child := lib.database.Task.Query().Where(task.TypeEQ("scrape")).OnlyX(ctx)
		input, err := tasks.DecodePayload[scrape.MetadataPayload](child.Payload)
		if err != nil || input.Code != film.Code || input.MovieID != film.ID {
			t.Fatalf("wrong scrape identity: %+v, %v", input, err)
		}
		progress, err := tasks.DecodePayload[domain.ScanPayload](lib.database.Task.GetX(ctx, job.ID).Payload)
		if err != nil || progress.Scan.Movies != 1 || progress.Scan.MatchedFiles != 2 ||
			progress.Scan.Stage != "done" || len(progress.ObsoleteCodes) != 0 {
			t.Fatalf("wrong scan progress: %+v, %v", progress, err)
		}
		if attempt == 0 && progress.Scan.RemovedMovies != 2 {
			t.Fatalf("old records were not counted: %+v", progress.Scan)
		}
	}
	for _, old := range []string{"ONSD-340A", "ONSD-340B"} {
		if _, err := os.Stat(export.EmbyMovieDir(cfg.EmbyDir, old)); !os.IsNotExist(err) {
			t.Fatalf("obsolete export %s remains: %v", old, err)
		}
	}
	film := lib.database.Movie.Query().WithFiles().OnlyX(ctx)
	var files []pan.File
	for _, file := range film.Edges.Files {
		files = append(files, pan.File{ID: file.FileID, Name: file.Name})
	}
	if _, err := export.WriteMedia(export.Config{EmbyDir: cfg.EmbyDir, PublicURL: cfg.PublicURL, STRMToken: ""}, film.Code, nfo.Movie{Code: film.Code}, files, nil, nil); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{videos[1].ID, videos[0].ID} {
		name := fmt.Sprintf("ONSD-340-cd%d.strm", i+1)
		body, err := os.ReadFile(filepath.Join(export.EmbyMovieDir(cfg.EmbyDir, film.Code), name))
		if err != nil || !strings.Contains(string(body), "/"+want+"\n") {
			t.Fatalf("incorrect part order in %s: %s, %v", name, body, err)
		}
	}
}

func TestScanLetterPartsValidatesDirectoryNFO(t *testing.T) {
	for _, code := range []string{"ONSD-340", "FJIN-106"} {
		t.Run(code, func(t *testing.T) {
			lib, client := panConcurrencyFixture(t)
			ctx, source := t.Context(), *lib.drive.Source()
			client.list = func(_ context.Context, _, id string, _, _ int) (pan.FilePage, error) {
				entries := []pan.File{{ID: "parts", Name: "ONSD-340", IsDirectory: true}}
				if id == "parts" {
					entries = []pan.File{
						{ID: "A", Name: "A.mp4", Size: domain.MinVideoSize},
						{ID: "B", Name: "B.mp4", Size: domain.MinVideoSize},
						{ID: "nfo", Name: "movie.nfo", PickCode: "nfo"},
					}
				}
				return pan.FilePage{Files: entries, Total: len(entries), Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
			}
			client.readMetadata = func(context.Context, string, string, int64) ([]byte, error) {
				return nfo.Encode(nfo.Movie{Code: code})
			}
			job := lib.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
			err := lib.Scan(ctx, tasks.Job{ID: job.ID, Payload: job.Payload})
			if code == "ONSD-340" {
				if err != nil {
					t.Fatal(err)
				}
				if film := lib.database.Movie.Query().WithFiles().OnlyX(ctx); film.Code != code || len(film.Edges.Files) != 2 {
					t.Fatalf("matching NFO did not identify both parts: %+v", film)
				}
			} else if err == nil || !strings.Contains(err.Error(), "不一致") || lib.database.Movie.Query().CountX(ctx) != 0 {
				t.Fatalf("conflicting NFO was allowed to merge parts: %v", err)
			}
		})
	}
}

func TestScanRetriesWholePartGroupWhenAnIdentityIsConfirmedConcurrently(t *testing.T) {
	lib, client := panConcurrencyFixture(t)
	ctx, source := t.Context(), *lib.drive.Source()
	entries := []pan.File{{ID: "A", Name: "ONSD-340A.mp4", Size: domain.MinVideoSize}}
	// Auxiliary files can otherwise push B into a different transaction.
	for i := range 101 {
		entries = append(entries, pan.File{ID: fmt.Sprintf("aux-%d", i), Name: "trailer.mp4", Size: 1})
	}
	entries = append(entries, pan.File{ID: "B", Name: "ONSD-340B.mp4", Size: domain.MinVideoSize},
		pan.File{ID: "nfo", Name: "movie.nfo", PickCode: "nfo"})
	variant := lib.database.Movie.Create().SetCode("ONSD-340B").SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
	lib.database.File.Create().SetFileID("B").SetName("ONSD-340B.mp4").SetSize(domain.MinVideoSize).
		SetAccountID(source.AccountID).SetRootID(source.Directory.ID).SetMovie(variant).ExecX(ctx)
	client.list = func(_ context.Context, _, id string, offset, limit int) (pan.FilePage, error) {
		files := entries
		if id == source.Directory.ID {
			files = []pan.File{{ID: "parts", Name: "ONSD-340", IsDirectory: true}}
		}
		end := min(offset+limit, len(files))
		return pan.FilePage{Files: files[offset:end], Total: len(files), HasMore: end < len(files),
			Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
	}
	client.readMetadata = func(context.Context, string, string, int64) ([]byte, error) {
		variant.Update().SetScrapeStatus(movie.ScrapeStatusDone).ExecX(ctx)
		return nfo.Encode(nfo.Movie{Code: "ONSD-340"})
	}
	job := lib.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	err := lib.Scan(ctx, tasks.Job{ID: job.ID, Payload: job.Payload})
	var retry *domain.RetryError
	if !errors.As(err, &retry) {
		t.Fatalf("expected an identity retry, got %v", err)
	}
	if lib.database.File.Query().CountX(ctx) != 1 || lib.database.Movie.Query().CountX(ctx) != 1 {
		t.Fatal("part of the group committed before the conflict was detected")
	}
	job = lib.database.Task.GetX(ctx, job.ID)
	if err := lib.Scan(ctx, tasks.Job{ID: job.ID, Payload: job.Payload}); err != nil {
		t.Fatal(err)
	}
	if lib.database.Movie.Query().CountX(ctx) != 2 || lib.database.Movie.Query().Where(movie.CodeEQ("ONSD-340")).ExistX(ctx) {
		t.Fatal("retry did not preserve separate release identities")
	}
}
