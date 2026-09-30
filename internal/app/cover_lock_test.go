package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/export"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/maintenance"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

type coverMediaProbe struct {
	scrapePkg.MetadataSource
	beforeMedia func()
}

func (probe coverMediaProbe) Image(ctx context.Context, image domain.ImageCandidate) (domain.Media, error) {
	probe.beforeMedia()
	return probe.MetadataSource.Image(ctx, image)
}

type coverExportProbe struct {
	export.MediaNotifier
	onUpdated func(context.Context, string) error
}

func (probe coverExportProbe) NotifyUpdatedTx(ctx context.Context, _ *ent.Tx, path string) error {
	return probe.onUpdated(ctx, path)
}

func TestCoverCleanupOnlyBlocksUntilArtworkCheckpoint(t *testing.T) {
	fixture := newPipelineFixture(t)
	ctx := t.Context()
	fixture.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	fixture.addCatalogueMovie(fixtureDetail())
	if _, err := fixture.library.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	{
		job, err := fixture.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindScan, tasks.KindScrape})
		if err != nil || job == nil {
			t.Fatalf("claim metadata: %+v, %v", job, err)
		}
		if err := fixture.library.Scan(ctx, *job); err != nil {
			t.Fatal(err)
		}
		if err := fixture.tasks.Queue().Finish(ctx, job.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	job, err := fixture.tasks.Queue().Claim(ctx, []tasks.Kind{tasks.KindScrape})
	if err != nil || job == nil {
		t.Fatalf("claim cover: %+v, %v", job, err)
	}
	cleanup, err := maintenance.New(fixture.dataDir, fixture.store.Client, fixture.images, fixture.scrape)
	if err != nil {
		t.Fatal(err)
	}
	unused, err := fixture.images.FromCover(fixtureJPEG(t, 60, 40))
	if err != nil {
		t.Fatal(err)
	}
	visited := make(map[string]bool)
	// Inject cleanup at each slow operation and on both sides of the database
	// handoff. These synchronous interleavings need no timing assumptions.
	checkCleanup := func(phase string, wantBusy bool) {
		t.Helper()
		visited[phase] = true
		_, err := cleanup.ClearCache(ctx)
		if wantBusy {
			if !errors.Is(err, maintenance.ErrCacheBusy) {
				t.Fatalf("%s: cleanup entered before checkpoint publication: %v", phase, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("%s: cleanup blocked outside image publication: %v", phase, err)
		}
		saved := fixture.store.Client.Task.GetX(ctx, job.ID)
		input, err := tasks.DecodePayload[scrapePkg.Payload](saved.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if input.Artwork == nil {
			if phase != "download" {
				t.Fatalf("%s: missing durable artwork checkpoint", phase)
			}
			return
		}
		for _, url := range []string{input.Artwork.Poster, input.Artwork.Fanart, input.Artwork.Thumbnail} {
			if _, err := fixture.images.ReadURL(url); err != nil {
				t.Fatalf("%s: cleanup removed cover artwork: %v", phase, err)
			}
		}
	}
	fixture.store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if body, ok := mutation.(*ent.TaskMutation).Payload(); ok {
				input, err := tasks.DecodePayload[scrapePkg.Payload](body)
				if err != nil {
					return nil, err
				}
				if input.Completed {
					// Movie references are still uncommitted on this connection.
					checkCleanup("complete", false)
				} else if input.Artwork != nil {
					checkCleanup("checkpoint", true)
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	stub := stubOf(t, fixture.driveService)
	list, info := stub.list, stub.info
	stub.list = func(ctx context.Context, token, id string, offset, limit int) (pan.FilePage, error) {
		checkCleanup("directory", false)
		return list(ctx, token, id, offset, limit)
	}
	stub.info = func(ctx context.Context, token, id string) (pan.FileInfo, error) {
		checkCleanup("info", false)
		return info(ctx, token, id)
	}
	service := scrapePkg.New(fixture.store.Client, fixture.driveService,
		coverMediaProbe{MetadataSource: fixtureMetadata{fixture.discover}, beforeMedia: func() { checkCleanup("download", false) }},
		fixture.images, fixture.tasks, scrapePkg.Dependencies{
			ExportManager: export.NewManager(export.Config{EmbyDir: fixture.embyDir, PublicURL: "http://127.0.0.1:8080"}),
			MediaNotifier: coverExportProbe{onUpdated: func(_ context.Context, path string) error {
				for _, name := range []string{"ABP-123.strm", "ABP-123.nfo", "poster.jpg", "fanart.jpg"} {
					if _, err := os.Stat(filepath.Join(path, name)); err != nil {
						return err
					}
				}
				checkCleanup("export", false)
				return nil
			}},
		})
	t.Cleanup(service.Close)
	if err := service.Scrape(ctx, *job); err != nil {
		t.Fatal(err)
	}
	if err := fixture.tasks.Queue().Finish(ctx, job.ID, nil); err != nil {
		t.Fatal(err)
	}
	checkCleanup("done", false)
	for _, phase := range []string{"download", "checkpoint", "directory", "info", "export", "complete", "done"} {
		if !visited[phase] {
			t.Errorf("phase %s was not checked", phase)
		}
	}
	if _, err := fixture.images.ReadURL(unused.Poster); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not remove unused artwork: %v", err)
	}
}
