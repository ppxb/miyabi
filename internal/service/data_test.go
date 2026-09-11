package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/ent/task"
	mediaimage "github.com/ppxb/miyabi/internal/image"
)

func dataFixture(t *testing.T) *DataService {
	t.Helper()
	directory := t.TempDir()
	store, err := database.Open(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	images, err := mediaimage.NewCache(directory)
	if err != nil {
		t.Fatal(err)
	}
	library := NewLibraryService(store.Client, nil, NewTaskService(store.Client), images)
	service, err := NewDataService(directory, NewScrapeService(library, nil, images))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func dataArtwork(t *testing.T, service *DataService, seed uint8) mediaimage.Artwork {
	t.Helper()
	cover := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := range 32 {
		for x := range 48 {
			cover.SetRGBA(x, y, color.RGBA{R: seed, G: uint8(x * 5), B: uint8(y * 7), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, cover); err != nil {
		t.Fatal(err)
	}
	artwork, err := service.scrape.images.FromCover(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return artwork
}

func artworkURLs(artwork mediaimage.Artwork) []string {
	return []string{artwork.Poster, artwork.Fanart, artwork.Thumbnail}
}

func TestDataCleanupPreservesAllLibraryAndUnfinishedTaskReferences(t *testing.T) {
	service := dataFixture(t)
	ctx := t.Context()
	db := service.scrape.library.database
	filmImages := dataArtwork(t, service, 10)
	additional := dataArtwork(t, service, 20)
	unused := dataArtwork(t, service, 30)
	film := db.Movie.Create().SetCode("ABP-001").SetTitle("Preserved film").SetWatched(true).
		SetCover(filmImages.Thumbnail).SetPoster(filmImages.Poster).
		SetFanarts([]string{filmImages.Fanart, additional.Fanart}).SaveX(ctx)
	// This movie has no files; another belongs to a different account/directory.
	other := db.Movie.Create().SetCode("ABP-002").SetCover(additional.Thumbnail).SetPoster(additional.Poster).SaveX(ctx)
	file := db.File.Create().SetFileID("other-video").SetName("video.mp4").SetSize(123).
		SetAccountID("other-account").SetRootID("other-directory").SetMovie(other).SaveX(ctx)
	history := db.WatchHistory.Create().SetAccountID("account").SetRootID("directory").SetMovie(film).
		SetSessionID("session").SetPosition(120).SetDuration(600).SaveX(ctx)
	if err := saveSetting(ctx, db, "preserved.setting", "unchanged"); err != nil {
		t.Fatal(err)
	}
	retained := append(artworkURLs(filmImages), artworkURLs(additional)...)
	for index, status := range []task.Status{task.StatusQueued, task.StatusRunning, task.StatusFailed} {
		artwork := dataArtwork(t, service, uint8(40+index*10))
		payload, err := encodeTaskPayload(coverPayload{Artwork: &artwork})
		if err != nil {
			t.Fatal(err)
		}
		db.Task.Create().SetType("cover").SetStatus(status).SetPayload(payload).ExecX(ctx)
		retained = append(retained, artworkURLs(artwork)...)
	}
	completed, err := encodeTaskPayload(coverPayload{Artwork: &unused})
	if err != nil {
		t.Fatal(err)
	}
	db.Task.Create().SetType("cover").SetStatus(task.StatusDone).SetPayload(completed).ExecX(ctx)
	db.Task.Create().SetType("cover").ExecX(ctx) // A job without generated artwork.

	before, err := service.Info(ctx)
	if err != nil || before.Cache.UnusedEntryCount != 3 || before.Cache.UnusedSizeBytes <= 0 {
		t.Fatalf("before cleanup: %+v %v", before, err)
	}
	var databaseSize int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(filepath.Join(service.directory, "miyabi.db"+suffix)); err == nil {
			databaseSize += info.Size()
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if before.DataDirectory != service.directory || !filepath.IsAbs(before.DataDirectory) || before.DatabaseSizeBytes != databaseSize || databaseSize == 0 {
		t.Fatalf("incorrect data directory or database size: %+v expected size=%d", before, databaseSize)
	}
	after, err := service.ClearCache(ctx)
	if err != nil || after.Cache.UnusedEntryCount != 0 || after.Cache.UnusedSizeBytes != 0 ||
		after.Cache.SizeBytes != before.Cache.SizeBytes-before.Cache.UnusedSizeBytes {
		t.Fatalf("after cleanup: %+v %v", after, err)
	}
	for _, url := range retained {
		if _, err := service.scrape.images.ReadURL(url); err != nil {
			t.Fatalf("referenced image was removed: %s %v", url, err)
		}
	}
	for _, url := range artworkURLs(unused) {
		if _, err := service.scrape.images.ReadURL(url); !os.IsNotExist(err) {
			t.Fatalf("unused image was not removed: %s %v", url, err)
		}
	}
	if got := db.Movie.GetX(ctx, film.ID); got.Title != film.Title || !got.Watched || *got.Cover != *film.Cover {
		t.Fatal("cache cleanup changed movie metadata")
	}
	if got := db.File.GetX(ctx, file.ID); got.FileID != file.FileID || got.Size != file.Size {
		t.Fatal("cache cleanup changed the file index")
	}
	if got := db.WatchHistory.GetX(ctx, history.ID); got.Position != 120 || got.SessionID != "session" {
		t.Fatal("cache cleanup changed watch progress")
	}
	if value, found, err := loadSetting[string](ctx, db, "preserved.setting"); err != nil || !found || value != "unchanged" {
		t.Fatalf("cache cleanup changed settings: %s %t %v", value, found, err)
	}
	if db.Task.Query().CountX(ctx) != 5 {
		t.Fatal("cache cleanup changed task records")
	}
	if again, err := service.ClearCache(ctx); err != nil || again.Cache != after.Cache {
		t.Fatalf("cleanup is not idempotent: %+v %v", again, err)
	}
}

func TestDataCleanupAndCoverWorkShareAnExclusiveGate(t *testing.T) {
	service := dataFixture(t)
	unused := dataArtwork(t, service, 70)
	if err := service.scrape.artwork.Lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ClearCache(t.Context()); !errors.Is(err, ErrCacheBusy) {
		t.Fatalf("cleanup entered an active artwork operation: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload, err := encodeTaskPayload(coverPayload{})
	if err != nil {
		t.Fatal(err)
	}
	// No drive is installed in the fixture. Cover must honor cancellation while
	// waiting for the gate, before attempting upstream access or creating files.
	if err := service.scrape.Cover(ctx, TaskJob{Payload: payload}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cover did not wait on the cleanup gate: %v", err)
	}
	service.scrape.artwork.Unlock()
	if _, err := service.ClearCache(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleanup error = %v", err)
	}
	for _, url := range artworkURLs(unused) {
		if _, err := service.scrape.images.ReadURL(url); err != nil {
			t.Fatal("blocked cleanup removed images")
		}
	}
	if _, err := service.ClearCache(t.Context()); err != nil {
		t.Fatalf("cleanup gate was not released: %v", err)
	}
}

func TestDataCleanupDoesNotDeleteWhenReferenceLookupFails(t *testing.T) {
	service := dataFixture(t)
	unused := dataArtwork(t, service, 80)
	if err := service.scrape.library.database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ClearCache(t.Context()); err == nil {
		t.Fatal("cleanup ignored an unreadable reference database")
	}
	for _, url := range artworkURLs(unused) {
		if _, err := service.scrape.images.ReadURL(url); err != nil {
			t.Fatal("failed reference lookup deleted images")
		}
	}
}
