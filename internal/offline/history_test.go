package offline

import (
	"context"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
)

func TestOfflineHistoryLoadsOnlyLatestTasksAndRetainsOldDownloads(t *testing.T) {
	service, pending, input, _ := offlineFixture(t)
	ctx := t.Context()
	service.database.OfflineDownload.UpdateOne(pending).SetProgress(17).ExecX(ctx)
	var latestID int
	for version := range 4 {
		var jobs []*ent.OfflineDownloadCreate
		for number := range 50 {
			payload := input
			payload.Hash, payload.InfoHash = fmt.Sprintf("history-%d", number), fmt.Sprintf("history-%d", number)
			jobs = append(jobs, createDownload(service.database, payload).SetStatus(offlinedownload.StatusDone).
				SetProgress(100))
		}
		records := service.database.OfflineDownload.CreateBulk(jobs...).SaveX(ctx)
		if version == 3 {
			latestID = records[len(records)-1].ID
		}
	}
	rowsRead := 0
	service.database.OfflineDownload.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, err := next.Query(ctx, query)
			if records, ok := value.([]*ent.OfflineDownload); ok {
				rowsRead += len(records)
			}
			return value, err
		})
	}))
	for _, read := range []func() ([]domain.OfflineSubmission, error){
		func() ([]domain.OfflineSubmission, error) {
			activity, err := service.Activity(ctx)
			return activity.Tasks, err
		},
		func() ([]domain.OfflineSubmission, error) { return service.Tasks(ctx, input.JavdbID, input.AccountID) },
	} {
		rowsRead = 0
		records, err := read()
		if err != nil || len(records) != 51 || rowsRead != 51 || records[0].TaskID != latestID {
			t.Fatalf("history was hydrated or limited before grouping: %d records, %d rows, %v", len(records), rowsRead, err)
		}
		old := records[len(records)-1]
		if old.TaskID != pending.ID || old.Status != string(offlinedownload.StatusRunning) || old.Progress != 17 || old.Phase != "downloading" {
			t.Fatalf("old download disappeared behind completed history: %#v", old)
		}
	}
}

func TestOfflineHistoryScopesMovieAndAccountBeforeGrouping(t *testing.T) {
	service, original, input, _ := offlineFixture(t)
	for _, change := range []func(*ent.OfflineDownload){
		func(payload *ent.OfflineDownload) { payload.JavdbID = "other-movie" },
		func(payload *ent.OfflineDownload) { payload.AccountID = "other-account" },
	} {
		payload := input
		change(&payload)
		createDownload(service.database, payload).SetStatus(offlinedownload.StatusDone).ExecX(t.Context())
	}
	records, err := service.Tasks(t.Context(), input.JavdbID, input.AccountID)
	if err != nil || len(records) != 1 || records[0].TaskID != original.ID {
		t.Fatalf("newer foreign task hid the movie's download: %#v, %v", records, err)
	}
}

func TestOfflineDownloadRejectsEmptyHash(t *testing.T) {
	service, _, input, _ := offlineFixture(t)
	input.Hash = ""
	if err := createDownload(service.database, input).Exec(t.Context()); err == nil {
		t.Fatal("empty download identity accepted")
	}
}
