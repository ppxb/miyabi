package library

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

type completedScanFixture struct {
	lib      *libraryTestService
	queued   domain.TaskInfo
	payload  domain.ScanPayload
	covered  *ent.Task
	snapshot *domain.MetadataSnapshot
	movie    *ent.Movie
	videos   []scan.Video
}

func newCompletedScanFixture(t *testing.T) *completedScanFixture {
	t.Helper()
	lib, queued, payload := libraryFixture(t)
	ctx := t.Context()
	videos := []scan.Video{fixtureVideo("101", "ABP-001.mp4")}
	if err := indexScanPage(ctx, lib, queued.ID, "baseline", "/Movies", videos, &payload); err != nil {
		t.Fatal(err)
	}
	record, err := lib.database.Movie.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := jpeg.Encode(&body, image.NewRGBA(image.Rect(0, 0, 6, 4)), nil); err != nil {
		t.Fatal(err)
	}
	artwork, err := lib.images.FromCover(body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.Update().SetScrapeStatus(movie.ScrapeStatusDone).
		SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &domain.MetadataSnapshot{
		AccountID: payload.Source.AccountID, DirectoryID: payload.Source.Directory.ID,
		Videos: scrape.VideoFingerprint([]pan.File{videos[0].File}), PosterVersion: mediaimage.PosterVersion,
	}
	record = record.Update().SetMetadataSnapshot(snapshot).SaveX(ctx)
	input := scrape.CoverPayload{
		MetadataPayload: scrape.MetadataPayload{Source: payload.Source, ScanTaskID: queued.ID, MovieID: record.ID, Code: record.Code},
		Artwork:         &artwork, Completed: true,
	}
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		t.Fatal(err)
	}
	covered, err := lib.database.Task.Create().SetType("cover").SetStatus(task.StatusDone).SetPayload(encoded).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.tasks.Queue().Finish(ctx, queued.ID, nil); err != nil {
		t.Fatal(err)
	}
	queued, err = lib.EnqueueScan(ctx, payload.Source)
	if err != nil {
		t.Fatal(err)
	}
	payload.Scan = domain.ScanProgress{Stage: "scanning"}
	return &completedScanFixture{lib: lib, queued: queued, payload: payload, movie: record, covered: covered, snapshot: snapshot,
		videos: videos,
	}
}

func TestRescanSchedulesOnlyChangedOrIncompleteMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*testing.T, *completedScanFixture)
		jobs   int
	}{
		{name: "unchanged"},
		{name: "outdated poster", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			f.snapshot.PosterVersion = 0
			f.movie.Update().SetMetadataSnapshot(f.snapshot).ExecX(t.Context())
		}},
		{name: "movie without snapshot", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			f.movie.Update().ClearMetadataSnapshot().ExecX(t.Context())
		}},
		{name: "deleted task history", change: func(t *testing.T, f *completedScanFixture) {
			f.lib.database.Task.DeleteOne(f.covered).ExecX(t.Context())
		}},
		{name: "new part", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) {
			video := fixtureVideo("102", "ABP-001-CD2.mp4")
			f.videos = append(f.videos, video)
		}},
		{name: "moved video", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) { f.videos[0].ParentID = "20" }},
		{name: "missing cache", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			if err := f.movie.Update().SetCover(mediaimage.URLPrefix + strings.Repeat("a", 64)).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "previous failure", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			if err := f.movie.Update().SetScrapeStatus(movie.ScrapeStatusFailed).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "another root snapshot", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			f.snapshot.DirectoryID = "other-root"
			f.movie.Update().SetMetadataSnapshot(f.snapshot).ExecX(t.Context())
		}},
		{name: "another account snapshot", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			f.snapshot.AccountID = "other-account"
			f.movie.Update().SetMetadataSnapshot(f.snapshot).ExecX(t.Context())
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newCompletedScanFixture(t)
			if scenario.change != nil {
				scenario.change(t, f)
			}
			if err := indexScanPage(t.Context(), f.lib, f.queued.ID, "rescan", "/Movies", f.videos, &f.payload); err != nil {
				t.Fatal(err)
			}
			if err := reconcileScan(t.Context(), f.lib, f.queued.ID, "rescan", &f.payload); err != nil {
				t.Fatal(err)
			}
			count, err := f.lib.database.Task.Query().Where(task.TypeEQ("scrape")).Count(t.Context())
			if err != nil || count != scenario.jobs {
				t.Fatalf("metadata jobs=%d want=%d err=%v", count, scenario.jobs, err)
			}
		})
	}
}
