package scan

import (
	"context"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent/movie"
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
			if got := canIdentifyVideo(tc.file); got != tc.valid {
				t.Fatalf("canIdentifyVideo(%+v) = %v, want %v", tc.file, got, tc.valid)
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

func TestApplyKnownCodesSkipsUnchangedFilesAndKeepsSharedDetection(t *testing.T) {
	ctx := t.Context()
	store, err := database.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	doneMovie := store.Client.Movie.Create().SetCode("ABP-001").SetScrapeStatus(movie.ScrapeStatusDone).SaveX(ctx)
	otherMovie := store.Client.Movie.Create().SetCode("IPX-123").SetScrapeStatus(movie.ScrapeStatusDone).SaveX(ctx)
	failedMovie := store.Client.Movie.Create().SetCode("SSIS-001").SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)
	manualMovie := store.Client.Movie.Create().SetCode("MIDE-001").SetManualCode("MIDE-001").SetScrapeStatus(movie.ScrapeStatusFailed).SaveX(ctx)

	store.Client.File.Create().
		SetFileID("v-done").SetName("ABP-001.mp4").SetSize(1 << 30).SetSha1("sha-done").
		SetAccountID("acc").SetRootID("root").SetMovie(doneMovie).ExecX(ctx)
	store.Client.File.Create().
		SetFileID("v-other").SetName("IPX-123.mp4").SetSize(1 << 30).SetSha1("sha-other").
		SetAccountID("acc").SetRootID("root").SetMovie(otherMovie).ExecX(ctx)
	store.Client.File.Create().
		SetFileID("v-failed").SetName("SSIS-001.mp4").SetSize(1 << 30).SetSha1("sha-failed").
		SetAccountID("acc").SetRootID("root").SetMovie(failedMovie).ExecX(ctx)
	store.Client.File.Create().
		SetFileID("v-manual").SetName("MIDE-001.mp4").SetSize(1 << 30).SetSha1("sha-manual").
		SetAccountID("acc").SetRootID("root").SetMovie(manualMovie).ExecX(ctx)

	t.Run("unchanged non-failed file skips NFO read", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "ABP-001"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo": body}}
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-done"}, Code: "ABP-001"},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, true); err != nil {
			t.Fatal(err)
		}
		if err := ResolveNFOCodes(ctx, sess, []pan.File{{ID: "n1", Name: "ABP-001.nfo", PickCode: "nfo"}}, videos); err != nil {
			t.Fatal(err)
		}
		if sess.reads != 0 || videos[0].Code != "ABP-001" {
			t.Fatalf("expected 0 NFO reads for unchanged file, got reads=%d video=%+v", sess.reads, videos[0])
		}
	})

	t.Run("changed file or failed movie still reads NFO", func(t *testing.T) {
		abpBody, _ := nfo.Encode(nfo.Movie{Code: "ABP-001"})
		ssisBody, _ := nfo.Encode(nfo.Movie{Code: "SSIS-001"})
		sess := &nfoStubSession{bodies: map[string][]byte{"abp": abpBody, "ssis": ssisBody}}
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-changed"}, Code: "ABP-001"},
			{File: pan.File{ID: "v-failed", Name: "SSIS-001.mp4", Size: 1 << 30, SHA1: "sha-failed"}, Code: "SSIS-001"},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, true); err != nil {
			t.Fatal(err)
		}
		sidecars := []pan.File{
			{ID: "n1", Name: "ABP-001.nfo", PickCode: "abp"},
			{ID: "n2", Name: "SSIS-001.nfo", PickCode: "ssis"},
		}
		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}
		if sess.reads != 2 {
			t.Fatalf("expected 2 NFO reads for changed + failed files, got %d", sess.reads)
		}
	})

	t.Run("known files still mark multi-movie directory as shared", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "ABP-001"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo": body}}
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-done"}, Code: "ABP-001"},
			{File: pan.File{ID: "v-other", Name: "IPX-123.mp4", Size: 1 << 30, SHA1: "sha-other"}, Code: "IPX-123"},
			{File: pan.File{ID: "v-new", Name: "feature.mp4", Size: 1 << 30}, Code: ""},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, true); err != nil {
			t.Fatal(err)
		}
		if err := ResolveNFOCodes(ctx, sess, []pan.File{{ID: "n1", Name: "movie.nfo", PickCode: "nfo"}}, videos); err != nil {
			t.Fatal(err)
		}
		if sess.reads != 0 || videos[2].Code != "" {
			t.Fatalf("shared directory should not assign single NFO to unnamed video: reads=%d video=%+v", sess.reads, videos[2])
		}
	})

	t.Run("preserveUnchanged false only skips manual codes", func(t *testing.T) {
		abpBody, _ := nfo.Encode(nfo.Movie{Code: "ABP-001"})
		mideBody, _ := nfo.Encode(nfo.Movie{Code: "MIDE-001"})
		sess := &nfoStubSession{bodies: map[string][]byte{"abp": abpBody, "mide": mideBody}}
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-done"}, Code: "ABP-001"},
			{File: pan.File{ID: "v-manual", Name: "MIDE-001.mp4", Size: 1 << 30, SHA1: "sha-changed"}, Code: "MIDE-001"},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, false); err != nil {
			t.Fatal(err)
		}
		sidecars := []pan.File{
			{ID: "n1", Name: "ABP-001.nfo", PickCode: "abp"},
			{ID: "n2", Name: "MIDE-001.nfo", PickCode: "mide"},
		}
		if err := ResolveNFOCodes(ctx, sess, sidecars, videos); err != nil {
			t.Fatal(err)
		}
		if sess.reads != 1 || !videos[1].Manual || !videos[1].known || videos[0].known {
			t.Fatalf("unexpected preserveUnchanged=false result: reads=%d videos=%+v", sess.reads, videos)
		}
	})

	t.Run("single movie folder uses known firstCode for new unnamed video conflict check", func(t *testing.T) {
		body, _ := nfo.Encode(nfo.Movie{Code: "IPX-999"})
		sess := &nfoStubSession{bodies: map[string][]byte{"nfo": body}}
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-done"}, Code: "ABP-001"},
			{File: pan.File{ID: "v-new", Name: "feature.mp4", Size: 1 << 30}, Code: ""},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, true); err != nil {
			t.Fatal(err)
		}
		err := ResolveNFOCodes(ctx, sess, []pan.File{{ID: "n1", Name: "movie.nfo", PickCode: "nfo"}}, videos)
		if err == nil || !strings.Contains(err.Error(), "不一致") || sess.reads != 1 {
			t.Fatalf("expected conflict against known firstCode, got err=%v reads=%d", err, sess.reads)
		}
	})

	t.Run("resolveTargetNFO short-circuits known unchanged video", func(t *testing.T) {
		videos := []Video{
			{File: pan.File{ID: "v-done", Name: "ABP-001.mp4", Size: 1 << 30, SHA1: "sha-done"}, Code: "ABP-001"},
		}
		if err := applyKnownCodes(ctx, store.Client, "acc", videos, true); err != nil {
			t.Fatal(err)
		}
		if err := resolveTargetNFO(ctx, nil, videos); err != nil {
			t.Fatalf("resolveTargetNFO should short-circuit without session calls: %v", err)
		}
	})
}
