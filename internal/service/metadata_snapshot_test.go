package service

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
)

type completedScanFixture struct {
	library *LibraryService
	queued  TaskInfo
	payload scanPayload
	covered *ent.Task
	input   coverPayload
	movie   *ent.Movie
	videos  []scanVideo
	entries map[string][]pan.File
}

func newCompletedScanFixture(t *testing.T) *completedScanFixture {
	t.Helper()
	library, queued, payload := libraryFixture(t)
	ctx := t.Context()
	videos := []scanVideo{fixtureVideo("101", "ABP-001.mp4")}
	if err := library.indexScanPage(ctx, queued.ID, "baseline", "/Movies", videos, &payload); err != nil {
		t.Fatal(err)
	}
	record, err := library.database.Movie.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := jpeg.Encode(&body, image.NewRGBA(image.Rect(0, 0, 6, 4)), nil); err != nil {
		t.Fatal(err)
	}
	artwork, err := library.images.FromCover(body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.Update().SetScrapeStatus(movie.ScrapeStatusDone).
		SetCover(artwork.Thumbnail).SetPoster(artwork.Poster).SetFanarts([]string{artwork.Fanart}).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nfo := pan.File{Name: "ABP-001.nfo", SHA1: strings.Repeat("1", 40)}
	poster := pan.File{Name: "ABP-001-poster.jpg", SHA1: strings.Repeat("2", 40)}
	fanart := pan.File{Name: "ABP-001-fanart.jpg", SHA1: strings.Repeat("3", 40)}
	input := coverPayload{metadataPayload: metadataPayload{Source: payload.Source, ScanTaskID: queued.ID, MovieID: record.ID, Code: record.Code},
		Artwork: &artwork, Snapshot: &metadataSnapshot{Videos: videoFingerprint([]pan.File{videos[0].File}),
			Directories: []metadataDirectorySnapshot{directorySnapshot("10", nfo, poster, fanart)},
		},
	}
	encoded, err := encodeTaskPayload(input)
	if err != nil {
		t.Fatal(err)
	}
	covered, err := library.database.Task.Create().SetType("cover").SetStatus(task.StatusDone).SetPayload(encoded).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := library.tasks.Finish(ctx, queued.ID, nil); err != nil {
		t.Fatal(err)
	}
	queued, err = library.tasks.enqueueScan(ctx, payload.Source)
	if err != nil {
		t.Fatal(err)
	}
	payload.Scan = ScanProgress{Stage: "scanning"}
	return &completedScanFixture{library: library, queued: queued, payload: payload, movie: record, covered: covered, input: input,
		videos: videos, entries: map[string][]pan.File{"10": {videos[0].File, nfo, poster, fanart}},
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
			encoded, err := encodeTaskPayload(f.input)
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
			encoded, err := encodeTaskPayload(f.input)
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
			if err := f.library.indexScanPage(t.Context(), f.queued.ID, "rescan", "/Movies", f.videos, &f.payload); err != nil {
				t.Fatal(err)
			}
			observed := make(scanObservations)
			for id, entries := range f.entries {
				observed.add(id, entries)
			}
			if err := f.library.reconcileScan(t.Context(), f.queued.ID, "rescan", &f.payload, observed); err != nil {
				t.Fatal(err)
			}
			count, err := f.library.database.Task.Query().Where(task.TypeEQ("scrape")).Count(t.Context())
			if err != nil || count != scenario.jobs {
				t.Fatalf("metadata jobs=%d want=%d err=%v", count, scenario.jobs, err)
			}
		})
	}
}
