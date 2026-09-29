package library

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/tasks"
)

var scanBenchmarkResult any

func BenchmarkUnchangedScanPage(b *testing.B) {
	lib, job, payload := libraryFixture(b)
	videos := make([]scan.Video, 100)
	for i := range videos {
		videos[i] = fixtureVideo(fmt.Sprint(i+1), fmt.Sprintf("ABP-%03d.mp4", i+1))
	}
	if err := indexScanPage(b.Context(), lib, job.ID, "initial", "/Movies", videos, &payload); err != nil {
		b.Fatal(err)
	}
	iteration := 0
	b.ReportAllocs()
	for b.Loop() {
		iteration++
		if err := indexScanPage(b.Context(), lib, job.ID, fmt.Sprint(iteration), "/Movies", videos, &payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanPayload(b *testing.B) {
	payload := domain.ScanPayload{
		Source: domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}},
		Scan:   domain.ScanProgress{Stage: "scanning", CurrentPath: "/Movies/fixture", FilesScanned: 50000, VideoFiles: 10000, DirectoriesDiscovered: 10000},
	}
	b.ReportAllocs()
	for b.Loop() {
		encoded, err := tasks.EncodePayload(payload)
		if err != nil {
			b.Fatal(err)
		}
		body, err := json.Marshal(encoded)
		if err != nil {
			b.Fatal(err)
		}
		scanBenchmarkResult = body
	}
}

func BenchmarkLibraryPage(b *testing.B) {
	lib, _, payload := libraryFixture(b)
	if err := ent.WithTx(b.Context(), lib.database, func(tx *ent.Tx) error {
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
		page, err := lib.Movies(b.Context(), 1, 24)
		if err != nil {
			b.Fatal(err)
		}
		scanBenchmarkResult = page
	}
}

func BenchmarkIdentifyAndIndexScanPage(b *testing.B) {
	lib, queued, payload := libraryFixture(b)
	videos := make([]scan.Video, 100)
	for i := range videos {
		videos[i] = fixtureVideo(fmt.Sprint(i), fmt.Sprintf("ABP-%03d.mp4", i))
	}
	if err := indexScanPage(b.Context(), lib, queued.ID, "initial", "/Movies", videos, &payload); err != nil {
		b.Fatal(err)
	}
	for _, film := range lib.database.Movie.Query().AllX(b.Context()) {
		film.Update().SetJavdbID(fmt.Sprint(film.ID)).ExecX(b.Context())
	}
	b.ReportAllocs()
	for b.Loop() {
		payload.ScanID = "rescan"
		if err := scan.ProcessScanPage(b.Context(), lib.database, queued.ID, "/Movies", videos, &payload,
			func(videos []scan.Video) []scan.Video { return videos }, lib.tasks); err != nil {
			b.Fatal(err)
		}
	}
}
