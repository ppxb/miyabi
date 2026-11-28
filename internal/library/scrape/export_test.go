package scrape

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/actor"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
)

func TestExportLocalMovieWritesCanonicalActorIdentity(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	profile := store.Client.ActorProfile.Create().
		SetMatchKey("森日向子").SetName("森日向子").SaveX(ctx)
	record := store.Client.Movie.Create().SetCode("ABP-900").SetMetadata(&nfo.Movie{
		Code: "ABP-900", Title: "Fixture",
		Actors: []nfo.Actor{{Provider: "fanza", ID: "f1", Name: "森日向子"}},
	}).SaveX(ctx)
	source := store.Client.Actor.Create().SetProvider("fanza").SetSourceID("f1").
		SetName("森日向子").AddMovieIDs(record.ID).SaveX(ctx)
	store.Client.Actor.Update().Where(actor.IDEQ(source.ID)).SetProfileID(profile.ID).ExecX(ctx)

	embyDir := t.TempDir()
	if _, err := ExportLocalMovie(ctx, store.Client, embyDir, "http://localhost", "", record, nil); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(export.EmbyMovieDir(embyDir, "ABP-900"), "ABP-900.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := nfo.Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	wantID := "miyabi-actor-" + strconv.Itoa(profile.ID)
	if len(doc.Actors) != 1 || doc.Actors[0].MediaServerID != wantID ||
		doc.Actors[0].Provider != "fanza" || doc.Actors[0].ID != "f1" {
		t.Fatalf("exported actors = %+v", doc.Actors)
	}
	if record.Metadata.Actors[0].MediaServerID != "" {
		t.Fatal("export rewrote stored movie metadata")
	}
}

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
		SetTitle("Index title").
		SetMetadata(&nfo.Movie{
			Code: "ALDN-613", Title: "兄嫁と中出ししまくった数日間 水野優香",
			Premiered: "2026-09-22", Runtime: 130, Rating: 4.69,
			Actors: []nfo.Actor{{Provider: "javdb", ID: "8VXx", Name: "水野優香", Gender: "female"}},
		}).
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

	// Export reads the saved document without loading actor or tag associations.
	loadedMovie, err := store.Client.Movie.Query().
		Where(movie.IDEQ(movieRecord.ID)).
		WithFiles().
		Only(t.Context())
	if err != nil {
		t.Fatalf("load movie: %v", err)
	}

	// Call ExportLocalMovie
	_, err = ExportLocalMovie(t.Context(), nil, embyDir, "http://127.0.0.1:8080", "testtoken", loadedMovie, images)
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
	_, err = ExportLocalMovie(t.Context(), nil, embyDir, "http://127.0.0.1:8080", "testtoken", loadedMovie, images)
	if err != nil {
		t.Fatalf("second ExportLocalMovie failed: %v", err)
	}
}

func TestExportLocalMovieRejectsMissingMetadataWithoutOverwritingFiles(t *testing.T) {
	root := t.TempDir()
	record := &ent.Movie{Code: "ABP-001", Title: "Index title", ScrapeStatus: movie.ScrapeStatusDone}
	dir := export.EmbyMovieDir(root, record.Code)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, record.Code+".nfo")
	if err := os.WriteFile(filename, []byte("saved document"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ExportLocalMovie(t.Context(), nil, root, "http://localhost", "", record, nil)
	if changed || !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("incomplete record was exported: %v %v", changed, err)
	}
	if body, err := os.ReadFile(filename); err != nil || string(body) != "saved document" {
		t.Fatalf("existing document changed: %s %v", body, err)
	}
}
