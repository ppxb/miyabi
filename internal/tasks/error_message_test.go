package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
)

func TestScrapeTasksStorePublicErrorsForRetriesAndFinalFailures(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const message = "刮削来源查询失败: pacopacomama，请检查网络和代理设置"
	failure := fmt.Errorf("prepare metadata: %w", domain.E(domain.KindUpstream, message, context.DeadlineExceeded))
	registry := NewRegistry()
	var received error
	registry.Register(NewHandler(KindScrape, nil, func(_ context.Context, _ *ent.Tx, _ Job, err error) (Change, error) {
		received = err
		return 0, nil
	}).WithRetry(domain.RetryDelay))
	s := NewService(store.Client, registry)
	for _, attempts := range []int{0, MaxRetries} {
		job := store.Client.Task.Create().SetType(string(KindScrape)).SetRetryCount(attempts).SaveX(t.Context())
		if err := s.Queue().Finish(t.Context(), job.ID, failure); err != nil {
			t.Fatal(err)
		}
		got := store.Client.Task.GetX(t.Context(), job.ID)
		if got.Error == nil || *got.Error != message {
			t.Fatalf("task exposed diagnostics: %+v", got.Error)
		}
		if attempts == 0 && (got.Status != task.StatusQueued || got.RetryAt == nil) {
			t.Fatal("retry was lost")
		}
		if attempts == MaxRetries && (got.Status != task.StatusFailed || received != failure) {
			t.Fatal("completion lost original error")
		}
	}
}
