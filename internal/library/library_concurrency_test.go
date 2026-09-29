package library

import (
	"context"
	"errors"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	drivePkg "github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestScanDiscardsLatePageAfterSourceChange(t *testing.T) {
	lib, client := panConcurrencyFixture(t)
	drive, ctx := lib.drive, t.Context()
	source := *drive.Source()
	queued := lib.database.Task.Query().Where(task.TypeEQ("scan")).OnlyX(ctx)
	payload := domain.ScanPayload{Source: source, Scan: domain.ScanProgress{Stage: "scanning"}}
	if err := indexScanPage(ctx, lib, queued.ID, "previous-scan", "/Movies", []scan.Video{fixtureVideo("101", "ABP-001.mp4")}, &payload); err != nil {
		t.Fatal(err)
	}
	queued = lib.database.Task.GetX(ctx, queued.ID)
	started := make(chan struct{}, 1)
	hold, release := panTestGate(t)
	client.list = func(context.Context, string, string, int, int) (pan.FilePage, error) {
		started <- struct{}{}
		<-hold
		return pan.FilePage{Total: 1, Files: []pan.File{fixtureVideo("late", "ABP-002.mp4").File},
			Path: []pan.Directory{{ID: source.Directory.ID, Name: source.Directory.Name}}}, nil
	}
	finished := make(chan error, 1)
	go func() { finished <- lib.Scan(ctx, tasks.Job{ID: queued.ID, Type: "scan", Payload: queued.Payload}) }()
	awaitPan(t, started)
	if err := drive.ClearDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	release()
	if err := awaitPan(t, finished); !errors.Is(err, drivePkg.ErrSourceChanged) {
		t.Fatalf("stale scan = %v", err)
	}
	files := lib.database.File.Query().AllX(ctx)
	if len(files) != 1 || files[0].FileID != "101" || files[0].ScanID != "previous-scan" {
		t.Fatalf("stale page indexed or pruned files: %+v", files)
	}
	if lib.database.Task.Query().Where(task.TypeEQ("scrape")).ExistX(ctx) {
		t.Fatal("stale scan enqueued metadata work")
	}
}
