package scan

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestExportedPreviewsAndCatalogueEntitiesSurviveLocalRescan(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	doc := nfo.Movie{
		Code: "ABP-123", Title: "Catalogue title", Zone: domain.ZoneCensored,
		IDs:    []nfo.UniqueID{{Type: "javdb", Value: "catalogue-id", Default: true}, {Type: "fanza", Value: "abp00123"}},
		Actors: []nfo.Actor{{Provider: "javdb", ID: "actor-id", Name: "Actor"}},
		Tags:   []nfo.Tag{{Provider: "javdb", ID: "tag-id", CategoryID: "category-id", Name: "Tag"}},
		Images: []domain.ImageCandidate{
			{Provider: "fanza", Role: "cover", URL: "https://official.example/cover.jpg", Layout: domain.CoverJacket},
			{Provider: "fanza", Role: "preview", URL: "https://official.example/sample.jpg?a=1&b=2"},
			{Provider: "javdb", Role: "preview", URL: "https://catalogue.example/sample.jpg"},
		},
		SelectedImage: domain.ImageCandidate{Provider: "fanza", Role: "cover", URL: "https://official.example/cover.jpg", Layout: domain.CoverJacket, Width: 2184, Height: 1468},
	}
	root := t.TempDir()
	if err := scrape.ExportEmbyMedia(root, "http://localhost:8080", "", doc.Code, doc,
		[]pan.File{{ID: "preview-video", Name: doc.Code + ".mp4"}}, testJPEG(t), testJPEG(t)); err != nil {
		t.Fatal(err)
	}
	store.Client.Movie.Create().SetCode(doc.Code).SetMetadata(&doc).SetScrapeStatus(movie.ScrapeStatusDone).SaveX(ctx)
	scanner := NewLocalScanner(store.Client, nil)
	for range 2 {
		if _, err := scanner.Scan(ctx, root); err != nil {
			t.Fatal(err)
		}
		film := store.Client.Movie.Query().Where(movie.CodeEQ(doc.Code)).WithActors().WithTags().OnlyX(ctx)
		if film.Metadata == nil || !reflect.DeepEqual(film.Metadata.Images, doc.Images) || film.Metadata.SelectedImage != doc.SelectedImage {
			t.Fatalf("local scan discarded image sources: %+v", film.Metadata)
		}
		if len(film.Edges.Actors) != 1 || film.Edges.Actors[0].Provider != "javdb" || film.Edges.Actors[0].SourceID != "actor-id" ||
			len(film.Edges.Tags) != 1 || film.Edges.Tags[0].Provider != "javdb" || film.Edges.Tags[0].SourceID != "tag-id" || film.Edges.Tags[0].CategoryID != "category-id" {
			t.Fatalf("local scan discarded catalogue search identities: %+v", film.Edges)
		}
	}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 100, 150))
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLocalScannerBasicSTRMWithSidecars(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	imgCache, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	movieDir := filepath.Join(tempDir, "IPX", "IPX-123")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. STRM file with Miyabi 302 endpoint
	strmContent := "http://127.0.0.1:8080/api/strm/play/115-test-file-999?token=secret123"
	if err := os.WriteFile(filepath.Join(movieDir, "IPX-123.strm"), []byte(strmContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. NFO file
	doc := nfo.Movie{
		Title: "Test Local Title",
		Code:  "IPX-123",
		IDs:   []nfo.UniqueID{{Type: "javdb", Default: true, Value: "javdb-ipx-123"}},
	}
	nfoBytes, err := nfo.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "IPX-123.nfo"), nfoBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Artwork
	jpegBytes := testJPEG(t)
	if err := os.WriteFile(filepath.Join(movieDir, "poster.jpg"), jpegBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "fanart.jpg"), jpegBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Subtitle
	if err := os.WriteFile(filepath.Join(movieDir, "IPX-123.vtt"), []byte("WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nHello"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewLocalScanner(store.Client, imgCache)
	res, err := scanner.Scan(ctx, tempDir)
	if err != nil {
		t.Fatalf("local scan failed: %v", err)
	}

	if res.MediaFiles != 1 || res.MoviesAdded != 1 || res.NFORead != 1 {
		t.Fatalf("unexpected scan result: %+v", res)
	}

	// Verify Movie record in DB
	film, err := store.Client.Movie.Query().Where(movie.CodeEQ("IPX-123")).Only(ctx)
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if film.Title != "Test Local Title" {
		t.Fatalf("unexpected title: %q", film.Title)
	}
	if film.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("unexpected scrape status: %v", film.ScrapeStatus)
	}
	if film.Poster == nil || film.Cover == nil || len(film.Fanarts) == 0 {
		t.Fatalf("artwork not cached on movie: poster=%v cover=%v fanarts=%v", film.Poster, film.Cover, film.Fanarts)
	}

	// Verify File record in DB
	f, err := store.Client.File.Query().Where(file.FileIDEQ("115-test-file-999")).Only(ctx)
	if err != nil {
		t.Fatalf("query file: %v", err)
	}
	if f.AccountID != "local" || f.RootID != "local" {
		t.Fatalf("unexpected file account/root: %s, %s", f.AccountID, f.RootID)
	}
	if f.MovieID == nil || *f.MovieID != film.ID {
		t.Fatalf("file not associated with movie: %v want %d", f.MovieID, film.ID)
	}

	// Verify Subtitle record in DB
	sub, err := store.Client.Subtitle.Query().Where(subtitle.MovieIDEQ(film.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("query subtitle: %v", err)
	}
	if sub.Name != "IPX-123.vtt" || sub.Source != "local" {
		t.Fatalf("unexpected subtitle: %+v", sub)
	}

	// Second run: idempotent
	res2, err := scanner.Scan(ctx, tempDir)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	if res2.MoviesAdded != 0 {
		t.Fatalf("expected 0 added movies on rescan, got %d", res2.MoviesAdded)
	}
}

func TestLocalScannerRawSTRMWithoutNFO(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	strmPath := filepath.Join(tempDir, "ABP-456.strm")
	if err := os.WriteFile(strmPath, []byte("https://example.com/stream/video.m3u8"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewLocalScanner(store.Client, nil)
	res, err := scanner.Scan(ctx, tempDir)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if res.MediaFiles != 1 || res.MoviesAdded != 1 || res.NFORead != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	film, err := store.Client.Movie.Query().Where(movie.CodeEQ("ABP-456")).Only(ctx)
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if film.ScrapeStatus != movie.ScrapeStatusPending {
		t.Fatalf("expected pending status, got %v", film.ScrapeStatus)
	}
}

func TestLocalScannerNFO_EmptyCodeFallsBackToMediaCode(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	movieDir := filepath.Join(tempDir, "MIDE", "MIDE-999")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}

	strmContent := "http://127.0.0.1:8080/api/strm/play/115-test-999?token=secret123"
	if err := os.WriteFile(filepath.Join(movieDir, "MIDE-999.strm"), []byte(strmContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// NFO without code / <num> tag
	doc := nfo.Movie{
		Title: "Local Title Without Num",
		Code:  "",
	}
	nfoBytes, err := nfo.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "MIDE-999.nfo"), nfoBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewLocalScanner(store.Client, nil)
	res, err := scanner.Scan(ctx, tempDir)
	if err != nil {
		t.Fatalf("local scan failed: %v", err)
	}

	if res.MediaFiles != 1 || res.MoviesAdded != 1 || res.NFORead != 1 {
		t.Fatalf("unexpected scan result: %+v", res)
	}

	film, err := store.Client.Movie.Query().Where(movie.CodeEQ("MIDE-999")).Only(ctx)
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if film.Title != "Local Title Without Num" {
		t.Fatalf("unexpected title: %q", film.Title)
	}
	if film.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("expected scrape status done, got %v", film.ScrapeStatus)
	}
}

func TestLocalScannerNFO_UnnormalizedCodeIsNormalized(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tempDir := t.TempDir()
	movieDir := filepath.Join(tempDir, "SSIS", "SSIS-555")
	if err := os.MkdirAll(movieDir, 0o755); err != nil {
		t.Fatal(err)
	}

	strmContent := "http://127.0.0.1:8080/api/strm/play/115-test-555?token=secret123"
	if err := os.WriteFile(filepath.Join(movieDir, "SSIS-555.strm"), []byte(strmContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// NFO with unnormalized lowercase code
	doc := nfo.Movie{
		Title: "Local Title Unnormalized",
		Code:  "ssis-555",
	}
	nfoBytes, err := nfo.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(movieDir, "SSIS-555.nfo"), nfoBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewLocalScanner(store.Client, nil)
	res, err := scanner.Scan(ctx, tempDir)
	if err != nil {
		t.Fatalf("local scan failed: %v", err)
	}

	if res.MediaFiles != 1 || res.MoviesAdded != 1 || res.NFORead != 1 {
		t.Fatalf("unexpected scan result: %+v", res)
	}

	film, err := store.Client.Movie.Query().Where(movie.CodeEQ("SSIS-555")).Only(ctx)
	if err != nil {
		t.Fatalf("query movie: %v", err)
	}
	if film.Title != "Local Title Unnormalized" {
		t.Fatalf("unexpected title: %q", film.Title)
	}
	if film.ScrapeStatus != movie.ScrapeStatusDone {
		t.Fatalf("expected scrape status done, got %v", film.ScrapeStatus)
	}
}
