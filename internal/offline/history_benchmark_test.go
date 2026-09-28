package offline

import (
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
)

var offlineBenchmarkResult any

func BenchmarkOfflineActivityHistory(b *testing.B) {
	service, _, _, payload := offlineFixture(b)
	if err := ent.WithTx(b.Context(), service.database, func(tx *ent.Tx) error {
		for batch := range 10 {
			var jobs []*ent.OfflineDownloadCreate
			for i := range 500 {
				input := ent.OfflineDownload{
					Code: "ABP-001", JavdbID: "movie", Hash: fmt.Sprintf("%040d", i%50),
					InfoHash: fmt.Sprintf("%040d", i%50), AccountID: payload.AccountID,
					DirectoryID: payload.Directory.ID,
				}
				jobs = append(jobs, createDownload(tx.Client(), input).SetStatus(offlinedownload.StatusDone).
					SetProgress(batch*10))
			}
			if err := tx.OfflineDownload.CreateBulk(jobs...).Exec(b.Context()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		activity, err := service.Activity(b.Context())
		if err != nil || len(activity.Tasks) != 50 {
			b.Fatalf("activity: %d tasks, %v", len(activity.Tasks), err)
		}
		offlineBenchmarkResult = activity
	}
}
