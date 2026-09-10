package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestPanDatabaseCommitDoesNotBlockPlaybackOrCanceledWaiters(t *testing.T) {
	play, source := playFixture(t)
	drive := play.library.drive
	playback, err := play.createSession(source, 0, []pan.PlaySource{{URL: "https://cdn.example/video", Height: 1080}})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	committed := make(chan error, 1)
	go func() {
		committed <- drive.commitSource(t.Context(), source, 0, func() error {
			started <- struct{}{}
			<-hold
			return nil
		})
	}()
	awaitPan(t, started)
	checked := make(chan error, 1)
	go func() {
		_, _, err := play.resource(playback.ID, 0)
		checked <- err
	}()
	if err := awaitPan(t, checked); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changed := make(chan error, 1)
	go func() { changed <- drive.ClearDirectory(ctx) }()
	cancel()
	if err := awaitPan(t, changed); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled commit waiter = %v", err)
	}
	release()
	if err := awaitPan(t, committed); err != nil {
		t.Fatal(err)
	}
}

func TestScanDiscardsLatePageAfterSourceChange(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	drive, ctx := library.drive, t.Context()
	source := drive.snapshot().source()
	queued := library.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	payload := scanPayload{Source: source, Scan: ScanProgress{Stage: "scanning"}}
	if err := library.indexScanPage(ctx, queued.ID, "previous-scan", "/Movies", []scanVideo{fixtureVideo("101", "ABP-001.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	queued = library.database.Task.GetX(ctx, queued.ID)
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		started <- struct{}{}
		<-hold
		return pan.FilePage{Total: 1, Files: []pan.File{fixtureVideo("late", "ABP-002.mp4").File},
			Path: []pan.Directory{{ID: source.Directory.ID, Name: source.Directory.Name}}}, nil
	}
	finished := make(chan error, 1)
	go func() { finished <- library.Scan(ctx, TaskJob{ID: queued.ID, Type: "scan", Payload: queued.Payload}) }()
	awaitPan(t, started)
	if err := drive.ClearDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	release()
	if err := awaitPan(t, finished); !errors.Is(err, errPanSourceChanged) {
		t.Fatalf("stale scan = %v", err)
	}
	files := library.database.File.Query().AllX(ctx)
	if len(files) != 1 || files[0].FileID != "101" || files[0].ScanID != "previous-scan" {
		t.Fatalf("stale page indexed or pruned files: %+v", files)
	}
	if library.database.Task.Query().Where(task.TypeEQ("scrape")).ExistX(ctx) {
		t.Fatal("stale scan enqueued metadata work")
	}
}

func TestMetadataSourceChangeAfterInfoPreventsUpload(t *testing.T) {
	library, client := panConcurrencyFixture(t)
	source := library.drive.snapshot().source()
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	video := pan.File{ID: "video", ParentID: source.Directory.ID, Name: "ABP-001.mp4"}
	client.info = func(context.Context, string, string) (pan.FileInfo, error) {
		started <- struct{}{}
		<-hold
		return pan.FileInfo{File: video, Path: []pan.Directory{{ID: source.Directory.ID}}}, nil
	}
	var uploads atomic.Int32
	client.uploadMetadata = func(context.Context, string, string, string, []byte) error {
		uploads.Add(1)
		return nil
	}
	finished := make(chan error, 1)
	go func() {
		scrape := &ScrapeService{library: library}
		finished <- scrape.uploadSidecar(t.Context(), source, 0, movieDirectory{
			ID: source.Directory.ID, Files: []pan.File{video}, VideoIDs: map[string]bool{video.ID: true},
		}, "movie.nfo", []byte("fixture"))
	}()
	awaitPan(t, started)
	if err := library.drive.ClearDirectory(t.Context()); err != nil {
		t.Fatal(err)
	}
	release()
	if err := awaitPan(t, finished); !errors.Is(err, errPanSourceChanged) {
		t.Fatalf("stale metadata upload = %v", err)
	}
	if uploads.Load() != 0 {
		t.Fatal("metadata was uploaded after the directory changed")
	}
}
