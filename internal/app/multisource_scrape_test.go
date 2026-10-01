package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/export"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/metadata"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

type workflowSource struct {
	id        string
	complete  bool
	body      []byte
	failImage bool
	queries   int
	images    int
}

func (s *workflowSource) ID() string {
	if s.id != "" {
		return s.id
	}
	return "fixture"
}
func (*workflowSource) Supports(string) bool { return true }
func (s *workflowSource) Fetch(_ context.Context, ref domain.MovieRef) (domain.MovieMetadata, error) {
	code := ref.Code
	s.queries++
	provider := s.ID()
	m := domain.MovieMetadata{Detail: domain.MovieDetail{Movie: domain.Movie{
		Code: code, Title: "Independent metadata", Sources: []domain.SourceID{{Provider: provider, ID: code}},
		Actors: []domain.Actor{{Provider: provider, ID: "42", Name: "Actor"}}, Tags: []domain.Tag{{Provider: provider, ID: "7", Name: "Tag"}},
	}}, Images: []domain.ImageCandidate{{Provider: provider, URL: "https://" + provider + ".example/cover.jpg", Role: "cover"}}}
	if s.complete {
		m.Detail.ReleaseDate, m.Detail.Duration = "2026-01-01", 120
		m.Detail.Maker = &domain.Maker{Provider: provider, ID: "maker", Name: "Studio"}
		m.Images = append(m.Images, domain.ImageCandidate{Provider: provider, URL: "https://" + provider + ".example/preview.jpg", Role: "preview"})
	}
	return m, nil
}
func (s *workflowSource) Media(context.Context, string) (domain.Media, error) {
	s.images++
	if s.failImage {
		return domain.Media{}, errors.New("image unavailable")
	}
	return domain.Media{Body: s.body, ContentType: "image/jpeg"}, nil
}

func TestJavDBMetadataAndArtworkCheckpointSurvivePublishFailure(t *testing.T) {
	f := newPipelineFixture(t)
	primary := &workflowSource{id: "fanza", complete: true, failImage: true}
	fallback := &workflowSource{id: "javdb", complete: true, body: f.catalogue.cover}
	meta, err := metadata.New(t.Context(), f.store.Client, primary, fallback)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(meta.Close)
	f.scrape = scrape.New(f.store.Client, f.driveService, meta, f.images, f.tasks, scrape.Dependencies{ExportManager: export.NewManager(export.Config{EmbyDir: f.embyDir, PublicURL: "http://127.0.0.1:8080"})})
	t.Cleanup(f.scrape.Close)
	failPublish := true
	f.store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if raw, ok := m.(*ent.TaskMutation).Payload(); ok && failPublish {
				p, err := tasks.DecodePayload[scrape.Payload](raw)
				if err == nil && p.Completed {
					return nil, errors.New("publish failed")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	f.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	if _, err := f.library.StartScan(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs := f.runQueue(t)
	job := f.store.Client.Task.GetX(t.Context(), jobs[1].ID)
	p, err := tasks.DecodePayload[scrape.Payload](job.Payload)
	if err != nil || job.Status != task.StatusFailed || p.Artwork == nil || p.Completed || p.Document.SelectedImage.Provider != "javdb" || p.Document.JavDBID() != "ABP-123" {
		t.Fatalf("fallback checkpoint=%+v err=%v", p, err)
	}
	failed := f.store.Client.Movie.Query().OnlyX(t.Context())
	if failed.JavdbID == nil || *failed.JavdbID != "ABP-123" || failed.Metadata.Actors[0].Provider != "javdb" {
		t.Fatal("catalogue metadata was not checkpointed before artwork")
	}
	if failed.Poster != nil || failed.MetadataSnapshot != nil || failed.ScrapeStatus != movie.ScrapeStatusFailed {
		t.Fatal("artwork escaped failed publish transaction")
	}
	failPublish = false
	job.Update().SetStatus(task.StatusQueued).ExecX(t.Context())
	f.runQueue(t)
	record := f.store.Client.Movie.Query().WithActors().WithTags().OnlyX(t.Context())
	if record.ScrapeStatus != movie.ScrapeStatusDone || record.JavdbID == nil || *record.JavdbID != "ABP-123" || record.Metadata.IDs[0].Type != "javdb" || record.Edges.Actors[0].Provider != "javdb" || record.Edges.Tags[0].Provider != "javdb" || record.Metadata.SelectedImage.Provider != "javdb" {
		t.Fatalf("catalogue metadata lost during publish: %+v", record)
	}
	detail, err := f.library.Movie(t.Context(), record.ID)
	if err != nil || len(detail.PreviewImages) != 2 || detail.Tags[0].Provider != "javdb" {
		t.Fatalf("saved detail lost previews or searchable tags: %+v %v", detail, err)
	}
	if primary.queries != 1 || fallback.queries != 1 || primary.images != 1 || fallback.images != 1 || len(f.catalogue.calls) != 0 {
		t.Fatalf("retry repeated work: primary=%+v fallback=%+v", primary, fallback)
	}
}

func TestMultiSourceScrapeWithoutJavDBResumesAfterImageFailure(t *testing.T) {
	f := newPipelineFixture(t)
	source := &workflowSource{body: f.catalogue.cover, failImage: true}
	meta, err := metadata.New(t.Context(), f.store.Client, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(meta.Close)
	service := scrape.New(f.store.Client, f.driveService, meta, f.images, f.tasks, scrape.Dependencies{ExportManager: export.NewManager(export.Config{EmbyDir: f.embyDir, PublicURL: "http://127.0.0.1:8080"})})
	t.Cleanup(service.Close)
	f.scrape = service
	f.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	if _, err := f.library.StartScan(t.Context()); err != nil {
		t.Fatal(err)
	}
	jobs := f.runQueue(t)
	if len(jobs) != 2 {
		t.Fatalf("workflow has %d jobs", len(jobs))
	}
	record := f.store.Client.Movie.Query().OnlyX(t.Context())
	job := f.store.Client.Task.GetX(t.Context(), jobs[1].ID)
	payload, err := tasks.DecodePayload[scrape.Payload](job.Payload)
	if err != nil || !payload.MetadataReady || payload.Completed || job.Status != task.StatusFailed || record.JavdbID != nil {
		t.Fatalf("metadata was not checkpointed: %+v %+v %v", job, payload, err)
	}
	source.failImage = false
	job.Update().SetStatus(task.StatusQueued).ExecX(t.Context())
	f.runQueue(t)
	record = f.store.Client.Movie.Query().WithActors().WithTags().OnlyX(t.Context())
	if record.ScrapeStatus != movie.ScrapeStatusDone || record.JavdbID != nil || record.Title != "Independent metadata" || record.Edges.Actors[0].Provider != "fixture" || record.Edges.Tags[0].Provider != "fixture" {
		t.Fatalf("source identity lost: %+v", record)
	}
	if source.queries != 1 || len(f.catalogue.calls) != 0 {
		t.Fatalf("retry re-queried metadata or JavDB: %d %v", source.queries, f.catalogue.calls)
	}
	body, err := os.ReadFile(filepath.Join(f.embyDir, "ABP", "ABP-123", "ABP-123.nfo"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := nfo.Decode(body)
	if err != nil || doc.JavDBID() != "" || doc.IDs[0].Type != "fixture" || doc.Actors[0].Provider != "fixture" {
		t.Fatalf("invalid NFO identities: %+v %v", doc, err)
	}
}

func TestMetadataCheckpointFailureRollsBackMovieAndTask(t *testing.T) {
	f := newPipelineFixture(t)
	f.drive.addFile("101", "10", "ABP-123.mp4", 2<<30, []byte("video"))
	f.addCatalogueMovie(fixtureDetail())
	if _, err := f.library.StartScan(t.Context()); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("metadata checkpoint rollback")
	f.store.Client.Task.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if raw, ok := m.(*ent.TaskMutation).Payload(); ok {
				p, err := tasks.DecodePayload[scrape.Payload](raw)
				if err == nil && p.MetadataReady {
					return nil, failure
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	jobs := f.runQueue(t)
	record := f.store.Client.Movie.Query().OnlyX(t.Context())
	checkpoint, err := tasks.DecodePayload[scrape.Payload](f.store.Client.Task.GetX(t.Context(), jobs[1].ID).Payload)
	if err != nil || checkpoint.MetadataReady || record.Title != "" || record.Metadata != nil || f.store.Client.Actor.Query().CountX(t.Context()) != 0 {
		t.Fatalf("metadata escaped rollback: %+v %+v %v", record, checkpoint, err)
	}
}
