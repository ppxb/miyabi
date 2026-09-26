package scrape

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)


func TestExportLocalMovie_ScrapedRecordExportsMissingSidecars(t *testing.T) {
	tempDir := t.TempDir()
	embyDir := filepath.Join(tempDir, "emby")
	imageDir := filepath.Join(tempDir, "images")

	store, err := database.Open(t.Context(), filepath.Join(tempDir, "data"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	images, err := mediaimage.NewCache(imageDir)
	if err != nil {
		t.Fatalf("create image cache: %v", err)
	}

	// Prepare valid cached image data (1x1 PNG)
	pngBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	artwork, err := images.Restore(pngBytes, pngBytes)
	if err != nil {
		t.Fatalf("restore artwork: %v", err)
	}


	// Create movie in database with ScrapeStatusDone
	releaseDate := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	movieRecord, err := store.Client.Movie.Create().
		SetCode("ALDN-613").
		SetTitle("兄嫁と中出ししまくった数日間 水野優香").
		SetScrapeStatus(movie.ScrapeStatusDone).
		SetPoster(artwork.Poster).
		SetFanarts([]string{artwork.Fanart}).
		SetReleaseDate(releaseDate).
		SetDuration(130).
		SetRating(4.69).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create movie: %v", err)
	}

	actorRecord, err := store.Client.Actor.Create().
		SetName("水野優香").
		SetJavdbID("8VXx").
		SetGender(actor.GenderFemale).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	_, err = movieRecord.Update().AddActors(actorRecord).Save(t.Context())
	if err != nil {
		t.Fatalf("add actor: %v", err)
	}

	_, err = store.Client.File.Create().
		SetFileID("3524256317969532243").
		SetParentID("root").
		SetName("ALDN-613.mp4").
		SetSha1("ABC123SHA1").
		SetSize(1024 * 1024 * 500).
		SetAccountID("100").
		SetScanID("scan-1").
		SetMovie(movieRecord).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create file: %v", err)
	}

	// Simulate fast STRM already written, but NFO and poster missing
	aldnDir := filepath.Join(embyDir, "ALDN", "ALDN-613")
	if err := os.MkdirAll(aldnDir, 0o755); err != nil {
		t.Fatalf("mkdir aldn: %v", err)
	}
	strmPath := filepath.Join(aldnDir, "ALDN-613.strm")
	if err := os.WriteFile(strmPath, []byte("http://127.0.0.1:8080/api/strm/play/3524256317969532243?token=testtoken\n"), 0o644); err != nil {
		t.Fatalf("write strm: %v", err)
	}

	// Load movie with files, actors, tags
	loadedMovie, err := store.Client.Movie.Query().
		Where(movie.IDEQ(movieRecord.ID)).
		WithFiles().
		WithActors().
		WithTags().
		Only(t.Context())
	if err != nil {
		t.Fatalf("load movie: %v", err)
	}

	// Call ExportLocalMovie
	err = ExportLocalMovie(embyDir, "http://127.0.0.1:8080", "testtoken", loadedMovie, images)
	if err != nil {
		t.Fatalf("ExportLocalMovie failed: %v", err)
	}

	// 1. Verify NFO exists and contains metadata
	nfoPath := filepath.Join(aldnDir, "ALDN-613.nfo")
	nfoContent, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("nfo not found: %v", err)
	}
	doc, err := nfo.Decode(nfoContent)
	if err != nil {
		t.Fatalf("decode nfo failed: %v", err)
	}
	if doc.Title != "兄嫁と中出ししまくった数日間 水野優香" || doc.Code != "ALDN-613" {
		t.Fatalf("nfo title or code mismatch: %+v", doc)
	}
	if len(doc.Actors) == 0 || doc.Actors[0].Name != "水野優香" {
		t.Fatalf("nfo actor mismatch: %+v", doc.Actors)
	}

	// 2. Verify poster exists
	posterPath := filepath.Join(aldnDir, "poster.jpg")
	pData, err := os.ReadFile(posterPath)
	if err != nil || len(pData) == 0 {
		t.Fatalf("poster data missing or empty: %v", err)
	}

	// 3. Verify fanart exists
	fanartPath := filepath.Join(aldnDir, "fanart.jpg")
	fData, err := os.ReadFile(fanartPath)
	if err != nil || len(fData) == 0 {
		t.Fatalf("fanart data missing or empty: %v", err)
	}


	// 4. Verify STRM still exists and has token
	sData, err := os.ReadFile(strmPath)
	if err != nil || !strings.Contains(string(sData), "token=testtoken") {
		t.Fatalf("strm mismatch: %s", string(sData))
	}

	// 5. Subsequent call returns nil cleanly without error (noop check)
	err = ExportLocalMovie(embyDir, "http://127.0.0.1:8080", "testtoken", loadedMovie, images)
	if err != nil {
		t.Fatalf("second ExportLocalMovie failed: %v", err)
	}
}

func TestExportEmbyMedia_WritesSTRMLast(t *testing.T) {
	tempDir := t.TempDir()
	embyDir := filepath.Join(tempDir, "emby")

	doc := nfo.Movie{
		Code:  "SSIS-999",
		Title: "Test Write Order",
	}
	videos := []pan.File{
		{ID: "vid-1", Name: "SSIS-999.mp4"},
	}
	poster := []byte("poster data")
	fanart := []byte("fanart data")

	err := ExportEmbyMedia(embyDir, "http://127.0.0.1:8080", "", "SSIS-999", doc, videos, poster, fanart)
	if err != nil {
		t.Fatalf("ExportEmbyMedia failed: %v", err)
	}

	movieDir := filepath.Join(embyDir, "SSIS", "SSIS-999")
	posterStat, err := os.Stat(filepath.Join(movieDir, "poster.jpg"))
	if err != nil {
		t.Fatalf("poster not found: %v", err)
	}
	nfoStat, err := os.Stat(filepath.Join(movieDir, "SSIS-999.nfo"))
	if err != nil {
		t.Fatalf("nfo not found: %v", err)
	}
	strmStat, err := os.Stat(filepath.Join(movieDir, "SSIS-999.strm"))
	if err != nil {
		t.Fatalf("strm not found: %v", err)
	}

	// STRM mod time must not be before poster or nfo
	if strmStat.ModTime().Before(posterStat.ModTime()) {
		t.Errorf("expected strm to be written after or equal to poster: strm=%v, poster=%v", strmStat.ModTime(), posterStat.ModTime())
	}
	if strmStat.ModTime().Before(nfoStat.ModTime()) {
		t.Errorf("expected strm to be written after or equal to nfo: strm=%v, nfo=%v", strmStat.ModTime(), nfoStat.ModTime())
	}
}

func TestRewriteSTRM(t *testing.T) {
	tempDir := t.TempDir()
	strmDir := filepath.Join(tempDir, "TEST-001")
	if err := os.MkdirAll(strmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	strmPath := filepath.Join(strmDir, "TEST-001.strm")
	oldContent := "http://127.0.0.1:8080/api/strm/play/12345?token=mytoken\n"
	if err := os.WriteFile(strmPath, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := RewriteSTRM(tempDir, "http://10.32.217.101:8080", "mytoken")
	if err != nil {
		t.Fatalf("RewriteSTRM error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file rewritten, got %d", count)
	}

	updated, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "http://10.32.217.101:8080/api/strm/play/12345?token=mytoken\n"
	if string(updated) != expected {
		t.Fatalf("expected %q, got %q", expected, string(updated))
	}

	// Idempotent test: second run rewrites 0 files
	count2, err := RewriteSTRM(tempDir, "http://10.32.217.101:8080", "mytoken")
	if err != nil || count2 != 0 {
		t.Fatalf("expected 0 files rewritten on second run, got %d (err: %v)", count2, err)
	}
}


