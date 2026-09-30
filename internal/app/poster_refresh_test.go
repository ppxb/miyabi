package app

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanRefreshesOutdatedPostersOnceWithoutDownloadingCovers(t *testing.T) {
	fixture := newPipelineFixture(t)
	ctx := t.Context()
	path := os.Getenv("MIYABI_TEST_FACE_IMAGE")
	if path == "" {
		t.Skip("set MIYABI_TEST_FACE_IMAGE to a real portrait outside the repository")
	}
	face, err := imaging.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cover := imaging.Paste(imaging.New(900, 600, color.White), imaging.Resize(face, 240, 0, imaging.Lanczos), image.Pt(30, 80))
	encode := func(img image.Image) []byte {
		t.Helper()
		var buffer bytes.Buffer
		if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	fixture.catalogue.cover = encode(cover)
	fixture.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	fixture.addCatalogueMovie(fixtureDetail()) // A censored title must also use face detection.
	if _, err := fixture.library.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.runQueue(t)

	// Reproduce the previously generated right-hand crop and an unversioned
	// snapshot, including the old poster already exported to Emby.
	old, err := fixture.images.Restore(encode(imaging.CropAnchor(cover, 400, 600, imaging.Right)), fixture.catalogue.cover)
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.store.Client.Movie.Query().OnlyX(ctx)
	snapshot := *record.MetadataSnapshot
	snapshot.PosterVersion = 0
	record.Update().SetPoster(old.Poster).SetFanarts([]string{old.Fanart}).SetCover(old.Thumbnail).SetMetadataSnapshot(&snapshot).ExecX(ctx)
	oldPoster, err := fixture.images.ReadURL(old.Poster)
	if err != nil {
		t.Fatal(err)
	}
	posterPath := filepath.Join(fixture.embyDir, "ABP", "ABP-123", "poster.jpg")
	if err := os.WriteFile(posterPath, oldPoster, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := maps.Clone(fixture.catalogue.calls)
	notifications := 0
	service := scrapePkg.New(fixture.store.Client, fixture.driveService, fixtureMetadata{fixture.discover}, fixture.images, fixture.tasks,
		scrapePkg.Dependencies{
			ExportManager: export.NewManager(export.Config{EmbyDir: fixture.embyDir, PublicURL: "http://127.0.0.1:8080"}),
			MediaNotifier: coverExportProbe{onUpdated: func(_ context.Context, path string) error {
				if path != filepath.Dir(posterPath) {
					t.Errorf("notified wrong directory: %s", path)
				}
				notifications++
				return nil
			}},
		})
	t.Cleanup(service.Close)
	fixture.scrape = service
	if _, err := fixture.library.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	jobs := fixture.runQueue(t)
	if len(jobs) != 2 || jobs[1].Type != tasks.KindScrape {
		t.Fatalf("expected scan/scrape refresh: %+v", jobs)
	}
	updated := fixture.store.Client.Movie.GetX(ctx, record.ID)
	if updated.MetadataSnapshot.PosterVersion != mediaimage.PosterVersion || *updated.Poster == old.Poster {
		t.Fatalf("poster did not upgrade: %+v", updated)
	}
	if *updated.Cover != old.Thumbnail || updated.Fanarts[0] != old.Fanart || updated.Title != record.Title {
		t.Fatal("poster refresh changed the full cover, thumbnail or metadata")
	}
	poster, err := os.ReadFile(posterPath)
	if err != nil || bytes.Equal(poster, oldPoster) {
		t.Fatalf("export still contains the old crop: %v", err)
	}
	want, err := fixture.images.ReadURL(*updated.Poster)
	if err != nil || !bytes.Equal(poster, want) || notifications != 1 {
		t.Fatalf("new poster was not exported and notified: notifications=%d, error=%v", notifications, err)
	}
	if !maps.Equal(calls, fixture.catalogue.calls) {
		t.Fatalf("refresh fetched upstream metadata or images: before=%v after=%v", calls, fixture.catalogue.calls)
	}
	if _, err := fixture.library.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	if jobs := fixture.runQueue(t); len(jobs) != 1 || jobs[0].Type != tasks.KindScan {
		t.Fatalf("unchanged poster was regenerated: %+v", jobs)
	}
}
