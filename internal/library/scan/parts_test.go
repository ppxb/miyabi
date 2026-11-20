package scan

import (
	"fmt"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/pan"
)

func partVideos(names ...string) []Video {
	videos := make([]Video, len(names))
	for i, name := range names {
		videos[i] = IdentifyVideo(pan.File{ID: fmt.Sprint(i), Name: name, Size: domain.MinVideoSize, SHA1: "hash"})
	}
	return videos
}

func TestDirectoryPartCode(t *testing.T) {
	for _, tc := range []struct {
		name, directory string
		files, sidecars []string
		want            string
	}{
		{"reported sample in reverse order", "onsd-340", []string{"onsd-340B.mp4", "onsd-340A.mp4"}, nil, "ONSD-340"},
		{"bare letters", "ONSD-340", []string{"a.mp4", "B.MKV", "c.avi"}, nil, "ONSD-340"},
		{"dash", "ONSD-340", []string{"ONSD-340-A.avi", "ONSD-340-B.avi"}, nil, "ONSD-340"},
		{"underscore", "ONSD-340", []string{"onsd340_a.mp4", "onsd340_b.mp4"}, nil, "ONSD-340"},
		{"dot", "ONSD-340", []string{"ONSD-340.A.wmv", "ONSD-340.B.wmv"}, nil, "ONSD-340"},
		{"space", "ONSD-340", []string{"ONSD-340 A.mp4", "ONSD-340 B.mp4"}, nil, "ONSD-340"},
		{"padding tolerance", "ONSD-340", []string{"ONSD00340A.mp4", "ONSD00340B.mp4"}, nil, "ONSD-340"},
		{"numeric catalogue", "060326_001", []string{"060326_001A.mp4", "060326_001B.mp4"}, nil, "060326_001"},
		{"matching sidecar", "ONSD-340", []string{"A.mp4", "B.mp4"}, []string{"onsd340.nfo"}, "ONSD-340"},
		{"generic sidecar", "ONSD-340", []string{"A.mp4", "B.mp4"}, []string{"movie.nfo"}, "ONSD-340"},
		{"no directory code", "Movies", []string{"FJIN-106A.mp4", "FJIN-106B.mp4"}, nil, ""},
		{"other directory code", "ONSD-340", []string{"FJIN-106A.mp4", "FJIN-106B.mp4"}, nil, ""},
		{"ambiguous directory", "ONSD-340 FJIN-106", []string{"A.mp4", "B.mp4"}, nil, ""},
		{"single variant", "ONSD-340", []string{"ONSD-340A.mp4"}, nil, ""},
		{"missing first part", "ONSD-340", []string{"ONSD-340B.mp4", "ONSD-340C.mp4"}, nil, ""},
		{"gap", "ONSD-340", []string{"A.mp4", "C.mp4"}, nil, ""},
		{"duplicate part", "ONSD-340", []string{"A.mp4", "A.mkv", "B.mp4"}, nil, ""},
		{"other movie", "ONSD-340", []string{"A.mp4", "B.mp4", "FJIN-106.mp4"}, nil, ""},
		{"unknown feature", "ONSD-340", []string{"A.mp4", "B.mp4", "feature.mp4"}, nil, ""},
		{"mixed stems", "ONSD-340", []string{"ONSD-340A.mp4", "B.mp4"}, nil, ""},
		{"mixed separators", "ONSD-340", []string{"ONSD-340_A.mp4", "ONSD-340-B.mp4"}, nil, ""},
		{"variant sidecar", "FJIN-106", []string{"FJIN-106A.mp4", "FJIN-106B.mp4"}, []string{"FJIN-106A.nfo"}, ""},
		{"conflicting sidecar", "ONSD-340", []string{"A.mp4", "B.mp4"}, []string{"FJIN-106.nfo"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sidecars []pan.File
			for _, name := range tc.sidecars {
				sidecars = append(sidecars, pan.File{Name: name})
			}
			if got := directoryPartCode("/Movies/"+tc.directory, partVideos(tc.files...), sidecars); got != tc.want {
				t.Fatalf("directoryPartCode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDirectoryPartsIgnoreAuxiliaryFiles(t *testing.T) {
	videos := partVideos("ONSD-340A.mp4", "ONSD-340B.mp4", "trailer.mp4")
	videos[2].Size = domain.MinVideoSize - 1
	if got := directoryPartCode("ONSD-340", videos, nil); got != "ONSD-340" {
		t.Fatalf("small auxiliary blocked parts: %q", got)
	}
	videos[1].Size = domain.MinVideoSize - 1
	if got := directoryPartCode("ONSD-340", videos, nil); got != "" {
		t.Fatalf("small auxiliary completed a part sequence: %q", got)
	}
}

func TestDirectoryPartsRespectExistingIdentities(t *testing.T) {
	for _, scenario := range []string{"manual", "renamed manual", "javdb", "scraped", "failed", "changed file", "other account", "known base"} {
		t.Run(scenario, func(t *testing.T) {
			run := reconcileFixture(t, 0, 0)
			ctx, db := t.Context(), run.scanner.db
			videos := partVideos("FJIN-106A.mp4", "FJIN-106B.mp4")
			builder := db.Movie.Create().SetCode("FJIN-106A")
			switch scenario {
			case "manual", "renamed manual":
				builder.SetManualCode("FJIN-106A")
			case "javdb":
				builder.SetScrapeStatus(movie.ScrapeStatusDone)
			case "changed file", "other account":
			case "scraped":
				builder.SetScrapeStatus(movie.ScrapeStatusDone)
			case "known base":
				builder.SetCode("FJIN-106")
			case "failed":
				builder.SetScrapeStatus(movie.ScrapeStatusFailed)
			}
			film := builder.SaveX(ctx)
			file := db.File.Create().SetFileID(videos[0].ID).SetName(videos[0].Name).
				SetSize(videos[0].Size).SetSha1(videos[0].SHA1).SetMovie(film).SetAccountID("account")
			if scenario == "renamed manual" {
				file.SetName("previous.mp4")
			}
			if scenario == "changed file" {
				file.SetSha1("previous-content")
			}
			if scenario == "other account" {
				file.SetAccountID("other")
			}
			file.ExecX(ctx)
			if err := resolveDirectoryParts(ctx, db, "account", "/Movies/FJIN-106", videos, nil); err != nil {
				t.Fatal(err)
			}
			grouped := scenario == "failed" || scenario == "changed file" || scenario == "other account" || scenario == "known base"
			for i, video := range videos {
				want := fmt.Sprintf("FJIN-106%c", 'A'+i)
				if grouped {
					want = "FJIN-106"
				}
				if video.Code != want {
					t.Fatalf("file %s: code = %s, want %s", video.Name, video.Code, want)
				}
			}
		})
	}
}
