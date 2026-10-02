package scrape

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/config"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestExportUsesResolvedApplicationDefaultsAndOverrides(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		name := "derived from data directory and listener"
		if explicit {
			name = "explicit directory and public URL"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("MIYABI_DATA_DIR", filepath.Join(root, "custom-data"))
			t.Setenv("MIYABI_LISTEN", ":9090")
			t.Setenv("MIYABI_LOG_LEVEL", "info")
			t.Setenv("MIYABI_EMBY_DIR", "")
			t.Setenv("MIYABI_PUBLIC_URL", "")
			t.Setenv("MIYABI_STRM_TOKEN", "token +/&")
			wantDir := filepath.Join(root, "custom-data", "emby")
			wantScheme, wantPort, wantPath := "http", "9090", "/api/strm/play/video-1"
			if explicit {
				wantDir = filepath.Join(root, "custom-export")
				wantScheme, wantPort, wantPath = "https", "9443", "/miyabi/api/strm/play/video-1"
				t.Setenv("MIYABI_EMBY_DIR", wantDir)
				t.Setenv("MIYABI_PUBLIC_URL", " https://media.example:9443/miyabi/// ")
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			doc := nfo.Movie{Code: "ABP-123", Title: "Saved title"}
			if err := ExportEmbyMedia(cfg.EmbyDir, cfg.PublicURL, cfg.STRMToken, doc.Code, doc,
				[]pan.File{{ID: "video-1", Name: "ABP-123.mp4"}}, nil, nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(wantDir, "ABP", doc.Code, doc.Code+".strm"))
			if err != nil {
				t.Fatal(err)
			}
			playURL, err := url.Parse(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			if playURL.Scheme != wantScheme || playURL.Hostname() == "" || playURL.Port() != wantPort ||
				playURL.Path != wantPath || playURL.Query().Get("token") != "token +/&" {
				t.Fatalf("incorrect exported playback URL: %s", playURL)
			}
			if explicit && playURL.Hostname() != "media.example" {
				t.Fatalf("explicit public host replaced: %s", playURL.Hostname())
			}
			if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
				t.Fatalf("unexpected fallback data directory: %v", err)
			}
		})
	}
}

func TestExportRejectsMissingDestinationsBeforeCreatingFiles(t *testing.T) {
	for _, tc := range []struct {
		name, directory, publicURL string
	}{
		{"missing directory", "", "http://localhost:9090"},
		{"blank directory", " \t", "http://localhost:9090"},
		{"missing URL", "exports", ""},
		{"blank URL", "exports", " \t"},
		{"slash-only URL", "exports", " / "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			doc := nfo.Movie{Code: "ABP-123", Title: "Saved title"}
			err := ExportEmbyMedia(tc.directory, tc.publicURL, "", doc.Code, doc,
				[]pan.File{{ID: "video-1", Name: "ABP-123.mp4"}}, []byte("poster"), []byte("fanart"))
			if err == nil {
				t.Fatal("missing export destination accepted")
			}
			entries, err := os.ReadDir(".")
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid export created files: %v, %v", entries, err)
			}
		})
	}
}

func TestLocalExportRejectsMissingURLWithoutChangingSidecars(t *testing.T) {
	root := t.TempDir()
	doc := &nfo.Movie{Code: "ABP-123", Title: "Updated title"}
	record := &ent.Movie{Code: doc.Code, Metadata: doc,
		Edges: ent.MovieEdges{Files: []*ent.File{{FileID: "video-1", Name: "ABP-123.mp4"}}}}
	dir := EmbyMovieDir(root, doc.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{"ABP-123.nfo", "ABP-123.strm", "poster.jpg", "fanart.jpg"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := ExportLocalMovie(root, "", "", record, nil)
	if err == nil || changed {
		t.Fatalf("incomplete configuration exported: changed = %t, error = %v", changed, err)
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "original "+name {
			t.Fatalf("existing %s changed: %q, %v", name, data, err)
		}
	}
}
