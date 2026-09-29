package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

type nfoStubSession struct {
	drive.Session
	bodies map[string][]byte
	reads  int
}

func (s *nfoStubSession) Read(_ context.Context, pickCode string, _ int64) ([]byte, error) {
	s.reads++
	return s.bodies[pickCode], nil
}

func TestResolveNFOCodesChecksContentAndReadsEachFileOnce(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		conflict   bool
	}{
		{name: "matching", code: "ABP-001"},
		{name: "conflicting content", code: "IPX-123", conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := nfo.Encode(nfo.Movie{Code: tc.code})
			if err != nil {
				t.Fatal(err)
			}
			sess := &nfoStubSession{bodies: map[string][]byte{"nfo": body}}
			videos := []Video{
				{File: pan.File{ID: "v1", Name: "ABP-001-CD1.mp4", Size: 1 << 30}, Code: "ABP-001"},
				{File: pan.File{ID: "v2", Name: "ABP-001-CD2.mp4", Size: 1 << 30}, Code: "ABP-001"},
			}
			err = ResolveNFOCodes(t.Context(), sess, []pan.File{{ID: "n1", Name: "ABP-001.nfo", PickCode: "nfo"}}, videos)
			if tc.conflict {
				if err == nil || !strings.Contains(err.Error(), "不一致") {
					t.Fatalf("conflict = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if sess.reads != 1 {
				t.Fatalf("NFO reads = %d", sess.reads)
			}
			for _, video := range videos {
				if video.Code != "ABP-001" {
					t.Fatalf("conflicting NFO replaced identity: %+v", video)
				}
			}
		})
	}
}

func TestResolveNFOCodesMatchesEachMovieInSharedDirectory(t *testing.T) {
	bodies := make(map[string][]byte)
	var sidecars []pan.File
	var videos []Video
	for _, code := range []string{"ABP-001", "IPX-123"} {
		body, err := nfo.Encode(nfo.Movie{Code: code})
		if err != nil {
			t.Fatal(err)
		}
		bodies[code] = body
		sidecars = append(sidecars, pan.File{ID: code, Name: code + ".nfo", PickCode: code})
		videos = append(videos, Video{File: pan.File{ID: code, Name: code + ".mp4", Size: 1 << 30}, Code: code})
	}
	sess := &nfoStubSession{bodies: bodies}
	if err := ResolveNFOCodes(t.Context(), sess, sidecars, videos); err != nil {
		t.Fatal(err)
	}
	if sess.reads != 2 || videos[0].Code != "ABP-001" || videos[1].Code != "IPX-123" {
		t.Fatalf("shared directory = %+v, reads = %d", videos, sess.reads)
	}
}

func TestCanIdentifyVideo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  pan.File
		valid bool
	}{
		{"valid mp4", pan.File{Name: "ABP-001.mp4", Size: 100 << 20}, true},
		{"valid mkv", pan.File{Name: "ABP-001.mkv", Size: 1 << 30}, true},
		{"directory", pan.File{Name: "ABP-001.mp4", Size: 1 << 30, IsDirectory: true}, false},
		{"non video", pan.File{Name: "ABP-001.nfo", Size: 1 << 30}, false},
		{"too small", pan.File{Name: "ABP-001.mp4", Size: (100 << 20) - 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanIdentifyVideo(tc.file); got != tc.valid {
				t.Fatalf("CanIdentifyVideo(%+v) = %v, want %v", tc.file, got, tc.valid)
			}
		})
	}
}

func TestResolveNFOCodes_ToleranceMatching(t *testing.T) {
	ctx := context.Background()

	t.Run("canonicalizes distributor prefix 200GANA to GANA", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "GANA-3458", Title: "Gana Title"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo-pick": body}}

		sidecars := []pan.File{
			{ID: "nfo-1", Name: "GANA-3458.nfo", PickCode: "nfo-pick"},
		}
		videos := []Video{
			{File: pan.File{ID: "v1", Name: "4k688.com@200GANA-3458.mp4", Size: 1 << 30}, Code: "200GANA-3458"},
			{File: pan.File{ID: "v2", Name: "APP.mp4", Size: 5 << 20}, Code: ""},
		}

		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}

		if videos[0].Code != "GANA-3458" {
			t.Fatalf("videos[0].Code = %q, want GANA-3458", videos[0].Code)
		}
		if videos[1].Code != "" {
			t.Fatalf("videos[1].Code = %q, want empty for auxiliary", videos[1].Code)
		}
	})

	t.Run("canonicalizes pure date CARIB to date format", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "060326-001", Title: "Carib Title"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo-pick": body}}

		sidecars := []pan.File{
			{ID: "nfo-1", Name: "060326-001.nfo", PickCode: "nfo-pick"},
		}
		videos := []Video{
			{File: pan.File{ID: "v1", Name: "Carib-060326-001.mp4", Size: 1 << 30}, Code: "CARIB-060326-001"},
		}

		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}

		if videos[0].Code != "060326-001" {
			t.Fatalf("videos[0].Code = %q, want 060326-001", videos[0].Code)
		}
	})

	t.Run("identifies unnamed feature video", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "ABP-001", Title: "ABP Title"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo-pick": body}}

		sidecars := []pan.File{
			{ID: "nfo-1", Name: "movie.nfo", PickCode: "nfo-pick"},
		}
		videos := []Video{
			{File: pan.File{ID: "v1", Name: "feature.mp4", Size: 1 << 30}, Code: ""},
			{File: pan.File{ID: "v2", Name: "trailer.mp4", Size: 10 << 20}, Code: ""},
		}

		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}

		if videos[0].Code != "ABP-001" {
			t.Fatalf("videos[0].Code = %q, want ABP-001", videos[0].Code)
		}
		if videos[1].Code != "" {
			t.Fatalf("videos[1].Code = %q, want empty for trailer", videos[1].Code)
		}
	})

	t.Run("conflict safety prevents cross-contamination", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "ABP-001", Title: "ABP Title"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo-pick": body}}

		sidecars := []pan.File{
			{ID: "nfo-1", Name: "movie.nfo", PickCode: "nfo-pick"},
		}
		videos := []Video{
			{File: pan.File{ID: "v1", Name: "ABP-001.mp4", Size: 1 << 30}, Code: "ABP-001"},
			{File: pan.File{ID: "v2", Name: "IPX-123.mp4", Size: 1 << 30}, Code: "IPX-123"},
		}

		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}

		// Because IPX-123 conflicts with ABP-001, neither is modified
		if videos[0].Code != "ABP-001" || videos[1].Code != "IPX-123" {
			t.Fatalf("videos unexpectedly modified: %+v", videos)
		}
	})

	t.Run("multi-NFO does not apply single NFO heuristic", func(t *testing.T) {
		sess := &nfoStubSession{}
		sidecars := []pan.File{
			{ID: "nfo-1", Name: "ABP-001.nfo"},
			{ID: "nfo-2", Name: "ABP-002.nfo"},
		}
		videos := []Video{
			{File: pan.File{ID: "v1", Name: "feature.mp4", Size: 1 << 30}, Code: ""},
		}

		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}

		if videos[0].Code != "" {
			t.Fatalf("multi-NFO folder should not identify unnamed video: got %q", videos[0].Code)
		}
	})
}
