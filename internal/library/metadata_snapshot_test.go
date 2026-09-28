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
	lib     *Service
	queued  domain.TaskInfo
	payload scan.Payload
	covered *ent.Task
	input   scrape.CoverPayload
	movie   *ent.Movie
	videos  []scan.Video
	entries map[string][]pan.File
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
	nfoFile := pan.File{Name: "ABP-001.nfo", SHA1: strings.Repeat("1", 40)}
	poster := pan.File{Name: "ABP-001-poster.jpg", SHA1: strings.Repeat("2", 40)}
	fanart := pan.File{Name: "ABP-001-fanart.jpg", SHA1: strings.Repeat("3", 40)}
	input := scrape.CoverPayload{
		MetadataPayload: scrape.MetadataPayload{Source: payload.Source, ScanTaskID: queued.ID, MovieID: record.ID, Code: record.Code},
		Artwork:         &artwork,
		Snapshot: &scrape.Snapshot{
			Videos:      scrape.VideoFingerprint([]pan.File{videos[0].File}),
			Directories: []scrape.DirectorySnapshot{scrape.NewDirectorySnapshot("10", nfoFile, poster, fanart)},
		},
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
	return &completedScanFixture{lib: lib, queued: queued, payload: payload, movie: record, covered: covered, input: input,
		videos: videos, entries: map[string][]pan.File{"10": {videos[0].File, nfoFile, poster, fanart}},
	}
}

func TestRescanSchedulesOnlyChangedOrIncompleteMetadata(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*testing.T, *completedScanFixture)
		jobs   int
	}{
		{name: "unchanged"},
		{name: "legacy cover without snapshot", jobs: 1, change: func(t *testing.T, f *completedScanFixture) {
			f.input.Snapshot = nil
			encoded, err := tasks.EncodePayload(f.input)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.covered.Update().SetPayload(encoded).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unrelated movie in shared directory", change: func(_ *testing.T, f *completedScanFixture) {
			f.entries["10"] = append(f.entries["10"], fixtureVideo("202", "ABP-002.mp4").File,
				pan.File{Name: "ABP-002.nfo", SHA1: strings.Repeat("4", 40)})
		}},
		{name: "edited NFO", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) { f.entries["10"][1].SHA1 = strings.Repeat("4", 40) }},
		{name: "edited artwork", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) { f.entries["10"][3].SHA1 = strings.Repeat("4", 40) }},
		{name: "missing artwork", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) { f.entries["10"] = f.entries["10"][:3] }},
		{name: "missing NFO", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) {
			f.entries["10"] = append(f.entries["10"][:1], f.entries["10"][2:]...)
		}},
		{name: "new part", jobs: 1, change: func(_ *testing.T, f *completedScanFixture) {
			video := fixtureVideo("102", "ABP-001-CD2.mp4")
			f.videos = append(f.videos, video)
			f.entries["10"] = append(f.entries["10"], video.File)
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
			f.input.Source.Directory.ID = "other-root"
			encoded, err := tasks.EncodePayload(f.input)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.covered.Update().SetPayload(encoded).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
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
			observed := make(scrape.DirectoryObservations)
			for id, entries := range f.entries {
				observed.Add(id, entries)
			}
			if err := reconcileScan(t.Context(), f.lib, f.queued.ID, "rescan", &f.payload, observed); err != nil {
				t.Fatal(err)
			}
			count, err := f.lib.database.Task.Query().Where(task.TypeEQ("scrape")).Count(t.Context())
			if err != nil || count != scenario.jobs {
				t.Fatalf("metadata jobs=%d want=%d err=%v", count, scenario.jobs, err)
			}
		})
	}
}
