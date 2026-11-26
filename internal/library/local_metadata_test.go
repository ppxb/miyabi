package library

import (
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	scrapePkg "github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestApplyLocalMetadataModeQueuesAfterPublishedCheckpoint(t *testing.T) {
	lib, _, _ := libraryFixture(t)
	ctx := t.Context()
	film := lib.database.Movie.Create().SetCode("ABP-001").SetScrapeStatus(movie.ScrapeStatusPending).SaveX(ctx)
	body, err := tasks.EncodePayload(scrapePkg.Payload{
		MetadataPayload: scrapePkg.MetadataPayload{
			Source:  domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10"}},
			MovieID: film.ID, Code: film.Code, ScanTaskID: 9,
		},
		Completed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	published := lib.database.Task.Create().SetType(tasks.KindScrape.String()).
		SetResourceKey(fmt.Sprintf("movie:%d", film.ID)).SetPayload(body).SaveX(ctx)

	local := domain.LibrarySource{
		AccountID: domain.LocalAccountID,
		Directory: domain.LibraryDirectory{ID: t.TempDir(), Name: embyLocalDirectoryName},
	}
	if err := lib.applyLocalMetadataMode(ctx, 7, domain.ScanPayload{Source: local}, []int{film.ID}); err != nil {
		t.Fatal(err)
	}
	jobs := lib.database.Task.Query().Where(task.TypeEQ(tasks.KindScrape.String())).AllX(ctx)
	if len(jobs) != 2 {
		t.Fatalf("scrape jobs=%d want 2: a published task must not block local metadata", len(jobs))
	}
	for _, job := range jobs {
		if job.ID == published.ID {
			continue
		}
		input, err := tasks.DecodePayload[scrapePkg.Payload](job.Payload)
		if err != nil || job.Status != task.StatusQueued || input.Source != local ||
			input.Rebuild || input.MovieID != film.ID || input.ScanTaskID != 7 || input.Code != film.Code {
			t.Fatalf("job=%+v payload=%+v err=%v", job, input, err)
		}
	}
}
