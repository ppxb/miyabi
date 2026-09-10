package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

var scanBenchmarkResult any

func BenchmarkUnchangedScanPage(b *testing.B) {
	library, job, payload := libraryFixture(b)
	videos := make([]scanVideo, 100)
	for i := range videos {
		videos[i] = fixtureVideo(fmt.Sprint(i+1), fmt.Sprintf("ABP-%03d.mp4", i+1))
	}
	if err := library.indexScanPage(b.Context(), job.ID, "initial", "/Movies", videos, &payload); err != nil {
		b.Fatal(err)
	}
	iteration := 0
	b.ReportAllocs()
	for b.Loop() {
		iteration++
		if err := library.indexScanPage(b.Context(), job.ID, fmt.Sprint(iteration), "/Movies", videos, &payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScanObservations(b *testing.B) {
	pages := make(map[string][]pan.File, 10000)
	for i := range 10000 {
		id := fmt.Sprint(i + 1)
		var entries []pan.File
		for j, name := range []string{"ABP-001-CD1.mp4", "ABP-001-CD2.mp4", "ABP-001.nfo", "poster.jpg", "fanart.jpg", "subdirectory"} {
			entries = append(entries, pan.File{
				ID: fmt.Sprintf("%d-%d", i, j), ParentID: id, Name: name, IsDirectory: j == 5,
				Size: 1024, PickCode: "fixture-pick-code", SHA1: "0123456789012345678901234567890123456789",
			})
		}
		pages[id] = entries
	}
	b.ReportAllocs()
	for b.Loop() {
		observed := make(scanObservations)
		for id, entries := range pages {
			observed.add(id, entries, 0)
		}
		scanBenchmarkResult = observed
	}
}

func BenchmarkScanPayload(b *testing.B) {
	payload := scanPayload{
		Source: LibrarySource{AccountID: "100", Directory: PanLibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"}},
		Scan:   ScanProgress{Stage: "scanning", CurrentPath: "/Movies/fixture", FilesScanned: 50000, VideoFiles: 10000, DirectoriesDiscovered: 10000},
	}
	b.ReportAllocs()
	for b.Loop() {
		body, err := json.Marshal(payload.taskPayload())
		if err != nil {
			b.Fatal(err)
		}
		scanBenchmarkResult = body
	}
}
