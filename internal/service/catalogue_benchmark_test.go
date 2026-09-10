package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/nfo"
)

var catalogueBenchmarkResult any

func BenchmarkLibraryPage(b *testing.B) {
	library, _, payload := libraryFixture(b)
	if err := ent.WithTx(b.Context(), library.database, func(tx *ent.Tx) error {
		label := tx.Tag.Create().SetJavdbID("tag").SetName("Fixture tag").SetCategoryID("category").SaveX(b.Context())
		for i := range 500 {
			film := tx.Movie.Create().SetCode(fmt.Sprintf("ABP-%04d", i)).SetTitle("Fixture title").AddTags(label).SaveX(b.Context())
			var files []*ent.FileCreate
			for part := range 10 {
				files = append(files, tx.File.Create().SetFileID(fmt.Sprintf("%d-%d", i, part)).
					SetName("video.mp4").SetSize(1024).SetAccountID(payload.Source.AccountID).
					SetRootID(payload.Source.Directory.ID).SetMovie(film))
			}
			if err := tx.File.CreateBulk(files...).Exec(b.Context()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		page, err := library.Movies(b.Context(), 1, 24)
		if err != nil {
			b.Fatal(err)
		}
		catalogueBenchmarkResult = page
	}
}

func BenchmarkOfflineActivityHistory(b *testing.B) {
	library, _, payload := libraryFixture(b)
	service := &OfflineService{database: library.database, tasks: library.tasks}
	if err := ent.WithTx(b.Context(), library.database, func(tx *ent.Tx) error {
		for batch := range 10 {
			var jobs []*ent.TaskCreate
			for i := range 500 {
				input, err := encodeTaskPayload(offlinePayload{
					Code: "ABP-001", JavDBID: "movie", Hash: fmt.Sprintf("%040d", i%50),
					InfoHash: fmt.Sprintf("%040d", i%50), AccountID: payload.Source.AccountID,
					DirectoryID: payload.Source.Directory.ID,
				})
				if err != nil {
					return err
				}
				jobs = append(jobs, tx.Task.Create().SetType("offline").SetStatus(task.StatusDone).
					SetProgress(batch*10).SetPayload(input))
			}
			if err := tx.Task.CreateBulk(jobs...).Exec(b.Context()); err != nil {
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
		catalogueBenchmarkResult = activity
	}
}

func BenchmarkTaskPayload(b *testing.B) {
	input := coverPayload{
		metadataPayload: metadataPayload{Source: LibrarySource{AccountID: "100", Directory: PanLibraryDirectory{ID: "10", Path: "/Movies"}},
			ScanTaskID: 1, MovieID: 2, Code: "ABP-001", JavDBID: "movie"},
		Document: nfo.Movie{Code: "ABP-001", Title: "Fixture title", Rating: 4.5},
		Snapshot: &metadataSnapshot{Videos: "fingerprint", Directories: []metadataDirectorySnapshot{{ID: "10"}}},
	}
	for i := range 20 {
		input.Document.Tags = append(input.Document.Tags, nfo.Tag{ID: fmt.Sprint(i), Name: "Fixture tag", CategoryID: "category"})
	}
	body, err := json.Marshal(input)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			encoded, err := encodeTaskPayload(input)
			if err != nil {
				b.Fatal(err)
			}
			stored, err := json.Marshal(encoded)
			if err != nil {
				b.Fatal(err)
			}
			catalogueBenchmarkResult = stored
		}
	})
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var record ent.Task
			if err := json.Unmarshal(body, &record.Payload); err != nil {
				b.Fatal(err)
			}
			decoded, err := decodeTaskPayload[coverPayload](record.Payload)
			if err != nil {
				b.Fatal(err)
			}
			catalogueBenchmarkResult = decoded
		}
	})
}

func BenchmarkIdentifyAndIndexScanPage(b *testing.B) {
	library, queued, payload := libraryFixture(b)
	videos := make([]scanVideo, 100)
	for i := range videos {
		videos[i] = fixtureVideo(fmt.Sprint(i), fmt.Sprintf("ABP-%03d.mp4", i))
	}
	if err := library.indexScanPage(b.Context(), queued.ID, "initial", "/Movies", videos, &payload); err != nil {
		b.Fatal(err)
	}
	for _, film := range library.database.Movie.Query().AllX(b.Context()) {
		film.Update().SetJavdbID(fmt.Sprint(film.ID)).ExecX(b.Context())
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := library.processScanPage(b.Context(), queued.ID, "rescan", "/Movies", videos, &payload,
			func(videos []scanVideo) []scanVideo { return videos }); err != nil {
			b.Fatal(err)
		}
	}
}
