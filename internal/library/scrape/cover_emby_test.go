package scrape

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestExportLocalMediaSingleVideo(t *testing.T) {
	tempDir := t.TempDir()
	service := &Service{}
	service.SetEmbyExport(tempDir, "http://192.168.1.50:8080", "my-token")

	doc := nfo.Movie{
		Code:  "IPX-123",
		Title: "Test Movie",
	}
	input := CoverPayload{
		MetadataPayload: MetadataPayload{
			Code:    "IPX-123",
			MovieID: 1,
		},
		Document: doc,
	}

	videos := []pan.File{
		{ID: "video-101", Name: "IPX-123.mp4"},
	}
	poster := []byte("fake poster data")
	fanart := []byte("fake fanart data")

	err := service.exportLocalMedia(t.Context(), input, doc, videos, poster, fanart)
	if err != nil {
		t.Fatalf("exportLocalMedia failed: %v", err)
	}

	movieDir := filepath.Join(tempDir, "IPX", "IPX-123")

	// 1. Check STRM
	strmPath := filepath.Join(movieDir, "IPX-123.strm")
	strmContent, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatalf("strm file not found: %v", err)
	}
	expectedSTRM := "http://192.168.1.50:8080/api/strm/play/video-101?token=my-token\n"
	if string(strmContent) != expectedSTRM {
		t.Fatalf("expected STRM content %q, got %q", expectedSTRM, string(strmContent))
	}

	// 2. Check NFO
	nfoPath := filepath.Join(movieDir, "IPX-123.nfo")
	nfoData, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("nfo file not found: %v", err)
	}
	parsedNFO, err := nfo.Decode(nfoData)
	if err != nil {
		t.Fatalf("decode nfo failed: %v", err)
	}
	if parsedNFO.Title != "Test Movie" || parsedNFO.Poster() != "poster.jpg" || parsedNFO.Fanart != "fanart.jpg" {
		t.Fatalf("nfo content mismatch: %+v", parsedNFO)
	}

	// 3. Check Poster and Fanart
	posterData, err := os.ReadFile(filepath.Join(movieDir, "poster.jpg"))
	if err != nil || string(posterData) != "fake poster data" {
		t.Fatalf("poster data mismatch: %v", err)
	}
	fanartData, err := os.ReadFile(filepath.Join(movieDir, "fanart.jpg"))
	if err != nil || string(fanartData) != "fake fanart data" {
		t.Fatalf("fanart data mismatch: %v", err)
	}
}

func TestExportLocalMediaMultiVideo(t *testing.T) {
	tempDir := t.TempDir()
	service := &Service{}
	service.SetEmbyExport(tempDir, "http://127.0.0.1:8080", "")

	doc := nfo.Movie{
		Code:  "SSIS-456",
		Title: "Two Disc Movie",
	}
	input := CoverPayload{
		MetadataPayload: MetadataPayload{
			Code:    "SSIS-456",
			MovieID: 2,
		},
		Document: doc,
	}

	videos := []pan.File{
		{ID: "video-cd1", Name: "SSIS-456-CD1.mp4"},
		{ID: "video-cd2", Name: "SSIS-456-CD2.mp4"},
	}

	err := service.exportLocalMedia(t.Context(), input, doc, videos, nil, nil)
	if err != nil {
		t.Fatalf("exportLocalMedia failed: %v", err)
	}

	movieDir := filepath.Join(tempDir, "SSIS", "SSIS-456")

	cd1Content, err := os.ReadFile(filepath.Join(movieDir, "SSIS-456-cd1.strm"))
	if err != nil {
		t.Fatalf("cd1 strm not found: %v", err)
	}
	if !strings.Contains(string(cd1Content), "/api/strm/play/video-cd1") {
		t.Fatalf("cd1 strm content mismatch: %s", string(cd1Content))
	}

	cd2Content, err := os.ReadFile(filepath.Join(movieDir, "SSIS-456-cd2.strm"))
	if err != nil {
		t.Fatalf("cd2 strm not found: %v", err)
	}
	if !strings.Contains(string(cd2Content), "/api/strm/play/video-cd2") {
		t.Fatalf("cd2 strm content mismatch: %s", string(cd2Content))
	}
}

func TestExportLocalMediaMultiVideoUnsorted(t *testing.T) {
	tempDir := t.TempDir()
	service := &Service{}
	service.SetEmbyExport(tempDir, "http://127.0.0.1:8080", "")

	doc := nfo.Movie{
		Code:  "SSIS-456",
		Title: "Two Disc Movie",
	}
	input := CoverPayload{
		MetadataPayload: MetadataPayload{
			Code:    "SSIS-456",
			MovieID: 2,
		},
		Document: doc,
	}

	// Pass in reversed order: cd2 first, then cd1
	videos := []pan.File{
		{ID: "video-cd2", Name: "SSIS-456-CD2.mp4"},
		{ID: "video-cd1", Name: "SSIS-456-CD1.mp4"},
	}

	err := service.exportLocalMedia(t.Context(), input, doc, videos, nil, nil)
	if err != nil {
		t.Fatalf("exportLocalMedia failed: %v", err)
	}

	movieDir := filepath.Join(tempDir, "SSIS", "SSIS-456")

	cd1Content, err := os.ReadFile(filepath.Join(movieDir, "SSIS-456-cd1.strm"))
	if err != nil {
		t.Fatalf("cd1 strm not found: %v", err)
	}
	if !strings.Contains(string(cd1Content), "/api/strm/play/video-cd1") {
		t.Fatalf("cd1 strm should point to video-cd1, got: %s", string(cd1Content))
	}

	cd2Content, err := os.ReadFile(filepath.Join(movieDir, "SSIS-456-cd2.strm"))
	if err != nil {
		t.Fatalf("cd2 strm not found: %v", err)
	}
	if !strings.Contains(string(cd2Content), "/api/strm/play/video-cd2") {
		t.Fatalf("cd2 strm should point to video-cd2, got: %s", string(cd2Content))
	}
}
