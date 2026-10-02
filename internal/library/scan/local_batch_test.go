package scan

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/embynotification"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
)

func localScannerFixture(t *testing.T) *LocalScanner {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	images, err := mediaimage.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewLocalScanner(store.Client, images)
}

func writeLocalFile(t *testing.T, root, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
		t.Fatal(err)
	}
}

type localNotifier func(context.Context, *ent.Tx, string) error

func (n localNotifier) NotifyUpdated(context.Context, string) error {
	return errors.New("local notifications must use the import transaction")
}

func (n localNotifier) NotifyUpdatedTx(ctx context.Context, tx *ent.Tx, path string) error {
	return n(ctx, tx, path)
}

func TestLocalScannerBatchesQueriesAndPreservesRemoteFileIdentity(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	count := 2*localScanBatchSize + 1
	for i := range count {
		writeLocalFile(t, root, fmt.Sprintf("TEST-%03d.strm", i), []byte(fmt.Sprintf("http://localhost/api/strm/play/file-%d", i)))
	}
	old := scanner.db.Movie.Create().SetCode("OLD-001").SaveX(ctx)
	remote := scanner.db.File.Create().SetFileID("file-0").SetName("remote-name.mp4").SetSize(1024).
		SetAccountID("account").SetRootID("remote-root").SetPath("original/path").SetMovieID(old.ID).SaveX(ctx)
	var movieQueries, fileQueries int
	scanner.db.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			switch query.(type) {
			case *ent.MovieQuery:
				movieQueries++
			case *ent.FileQuery:
				fileQueries++
			}
			return next.Query(ctx, query)
		})
	}))
	for run := range 2 {
		movieQueries, fileQueries = 0, 0
		result, err := scanner.Scan(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if movieQueries != 3 || fileQueries != 3 {
			t.Fatalf("run %d: movie queries=%d file queries=%d, want 3 each for %d files", run, movieQueries, fileQueries, count)
		}
		wantAdded := count
		if run == 1 {
			wantAdded = 0
		}
		if result.MediaFiles != count || result.MoviesAdded != wantAdded || result.FilesScanned != count {
			t.Fatalf("run %d: result=%+v", run, result)
		}
		got := scanner.db.File.GetX(ctx, remote.ID)
		film := scanner.db.Movie.Query().Where(movie.CodeEQ("TEST-000")).OnlyX(ctx)
		if got.AccountID != remote.AccountID || got.RootID != remote.RootID || got.Name != remote.Name ||
			got.Size != remote.Size || got.Path != remote.Path || got.MovieID == nil || *got.MovieID != film.ID {
			t.Fatalf("import changed remote file identity or missed association: %+v", got)
		}
		if got := scanner.db.File.Query().CountX(ctx); got != count {
			t.Fatalf("file count=%d, want %d", got, count)
		}
	}
}

func TestLocalImportKeepsManualIdentityWhenOldExportStillExists(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	record := scanner.db.Movie.Create().SetCode("IPX-123").SetManualCode("IPX-123").
		SetTitle("corrected").SetScrapeStatus(movie.ScrapeStatusFailed).SetCover("correct-cover").SaveX(ctx)
	remote := scanner.db.File.Create().SetFileID("remote-1").SetName("ABP-001.mp4").SetSize(1 << 30).
		SetAccountID("account").SetRootID("root").SetMovieID(record.ID).SaveX(ctx)
	writeLocalFile(t, root, "ABP-001.strm", []byte("http://localhost/api/strm/play/remote-1"))
	body, err := nfo.Encode(nfo.Movie{Code: "ABP-001", Title: "stale"})
	if err != nil {
		t.Fatal(err)
	}
	writeLocalFile(t, root, "ABP-001.nfo", body)
	if _, err := scanner.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	got := scanner.db.Movie.GetX(ctx, record.ID)
	if got.Code != "IPX-123" || got.Title != "corrected" || got.ScrapeStatus != movie.ScrapeStatusFailed || got.Cover == nil || *got.Cover != "correct-cover" || scanner.db.Movie.Query().CountX(ctx) != 1 {
		t.Fatalf("stale NFO replaced corrected metadata: %+v", got)
	}
	if got := scanner.db.File.GetX(ctx, remote.ID); got.MovieID == nil || *got.MovieID != record.ID || got.Name != remote.Name {
		t.Fatal("stale export reassigned the corrected movie's video")
	}
}

func TestLocalBatchObservesNFOIdentityAndRankingChanges(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	old := scanner.db.Movie.Create().SetCode("AAA-001").SaveX(ctx)
	decorated := scanner.db.Movie.Create().SetCode("200GANA-3458").SaveX(ctx)
	scanner.db.Movie.Create().SetCode("GANA-3458").ExecX(ctx)
	for _, entry := range []struct{ stem, code, id string }{
		{"AAA-001-01", "ZZZ-009", "renamed"},
		{"AAA-001-02", "AAA-001", ""},
		{"ZZZ-009", "ZZZ-009", "renamed"},
		{"200GANA-3458", "200GANA-3458", "scraped"},
		{"GANA-3458", "", ""},
	} {
		writeLocalFile(t, root, entry.stem+".strm", []byte("http://localhost/api/strm/play/"+entry.stem))
		if entry.code != "" {
			doc := nfo.Movie{Code: entry.code}
			if entry.id != "" {
				doc.IDs = []nfo.UniqueID{{Type: "javdb", Default: true, Value: entry.id}}
			}
			body, err := nfo.Encode(doc)
			if err != nil {
				t.Fatal(err)
			}
			writeLocalFile(t, root, entry.stem+".nfo", body)
		}
	}
	result, err := scanner.Scan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.MoviesAdded != 1 || result.NFORead != 4 || result.MediaFiles != 5 {
		t.Fatalf("unexpected result after NFO identity changes: %+v", result)
	}
	for _, id := range []string{"AAA-001-01", "ZZZ-009"} {
		record := scanner.db.File.Query().Where(file.FileIDEQ(id)).OnlyX(ctx)
		if record.MovieID == nil || *record.MovieID != old.ID {
			t.Fatalf("renamed movie was not reused for %s: %+v", id, record)
		}
	}
	remaining := scanner.db.File.Query().Where(file.FileIDEQ("AAA-001-02")).OnlyX(ctx)
	if remaining.MovieID == nil || *remaining.MovieID == old.ID {
		t.Fatal("old lookup bucket retained the renamed movie")
	}
	gana := scanner.db.File.Query().Where(file.FileIDEQ("GANA-3458")).OnlyX(ctx)
	if gana.MovieID == nil || *gana.MovieID != decorated.ID {
		t.Fatal("batch did not observe the newly scraped candidate's priority")
	}
}

func TestLocalBatchCommitRollbackAndCancellation(t *testing.T) {
	for _, outcome := range []string{"commit", "failure", "cancel", "second batch failure"} {
		t.Run(outcome, func(t *testing.T) {
			scanner := localScannerFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			root := t.TempDir()
			writeLocalFile(t, root, "poster.jpg", testJPEG(t))
			count := 2
			if outcome == "second batch failure" {
				count = localScanBatchSize + 1
			}
			for i := range count {
				stem := fmt.Sprintf("TEST-%03d", i)
				writeLocalFile(t, root, stem+".mp4", []byte("video"))
				writeLocalFile(t, root, stem+".srt", []byte("subtitle"))
			}
			failure := errors.New("notification persistence failed")
			calls := 0
			scanner.SetMediaNotifier(localNotifier(func(ctx context.Context, tx *ent.Tx, path string) error {
				calls++
				if err := tx.EmbyNotification.Create().SetPath(path).OnConflictColumns(embynotification.FieldPath).UpdateNewValues().Exec(ctx); err != nil {
					return err
				}
				// Readers on another connection must see only previously committed batches.
				wantVisible := (calls - 1) * localScanBatchSize
				if got := scanner.db.File.Query().CountX(ctx); got != wantVisible {
					t.Errorf("uncommitted files visible: %d, want %d", got, wantVisible)
				}
				if got := scanner.db.EmbyNotification.Query().CountX(ctx); got != calls-1 {
					t.Errorf("uncommitted notification visible: %d, want %d", got, calls-1)
				}
				if outcome == "cancel" {
					cancel()
					return ctx.Err()
				}
				if outcome == "failure" || outcome == "second batch failure" && calls == 2 {
					return failure
				}
				return nil
			}))
			_, err := scanner.Scan(ctx, root)
			want := 0
			switch outcome {
			case "commit":
				want = count
				if err != nil {
					t.Fatal(err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			default:
				if !errors.Is(err, failure) {
					t.Fatalf("notification failure lost: %v", err)
				}
				if calls == 2 {
					want = localScanBatchSize
				}
			}
			if files, movies, subs := scanner.db.File.Query().CountX(t.Context()), scanner.db.Movie.Query().CountX(t.Context()), scanner.db.Subtitle.Query().CountX(t.Context()); files != want || movies != want || subs != want {
				t.Fatalf("partial batch: files=%d movies=%d subtitles=%d, want %d each", files, movies, subs, want)
			}
			wantNotifications := 0
			if want > 0 {
				wantNotifications = 1
			}
			if got := scanner.db.EmbyNotification.Query().CountX(t.Context()); got != wantNotifications {
				t.Fatalf("notifications=%d, want %d", got, wantNotifications)
			}
			if !scanner.images.TryLockArtwork() {
				t.Fatal("artwork lock leaked after batch completion")
			}
			scanner.images.UnlockArtwork()
		})
	}
}

func TestLocalArtworkPreparationAllowsWritesAndBlocksPruningUntilCommit(t *testing.T) {
	scanner := localScannerFixture(t)
	ctx, root := t.Context(), t.TempDir()
	scraper := scrape.New(scanner.db, nil, nil, scanner.images, nil, scrape.Dependencies{})
	t.Cleanup(scraper.Close)
	magic := "local-scan-test:" + root // Unique even when this test is run repeatedly.
	decoded := 0
	image.RegisterFormat("local-scan-test", magic, func(io.Reader) (image.Image, error) {
		decoded++
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := scanner.db.Movie.Create().SetCode("CONCURRENT-001").Exec(writeCtx); err != nil {
			t.Errorf("image decoder held the database writer: %v", err)
		}
		if scraper.TryLockArtwork() {
			scraper.UnlockArtwork()
			t.Error("cache maintenance could run during local artwork preparation")
		}
		return image.NewRGBA(image.Rect(0, 0, 16, 24)), nil
	}, nil)
	writeLocalFile(t, root, "TEST-001.strm", []byte("http://localhost/api/strm/play/local"))
	writeLocalFile(t, root, "poster.jpg", []byte(magic))
	notified := false
	scanner.SetMediaNotifier(localNotifier(func(ctx context.Context, tx *ent.Tx, path string) error {
		notified = true
		if scraper.TryLockArtwork() {
			scraper.UnlockArtwork()
			t.Error("cache protection ended before movie references committed")
		}
		return tx.EmbyNotification.Create().SetPath(path).Exec(ctx)
	}))
	if _, err := scanner.Scan(ctx, root); err != nil {
		t.Fatal(err)
	}
	if decoded != 1 || !notified {
		t.Fatalf("test missed image decoding or commit: decoded=%d notified=%t", decoded, notified)
	}
	film := scanner.db.Movie.Query().Where(movie.CodeEQ("TEST-001")).OnlyX(ctx)
	if exists, err := scanner.images.Exists(scrape.MovieArtwork(film)); err != nil || !exists {
		t.Fatalf("committed artwork missing: %v", err)
	}
	if !scraper.TryLockArtwork() {
		t.Fatal("artwork lock leaked after commit")
	}
	scraper.UnlockArtwork()
}
