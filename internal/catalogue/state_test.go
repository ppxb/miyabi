package catalogue

import (
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

func TestMovieStatesScopeWorkflowsToSource(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()

	source := domain.LibrarySource{
		AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	knownJavDBID := "fixture-movie"
	local := &stubLocalState{
		source: &source,
		movies: nil, // initially not in library
	}

	// Create an active download in the mounted source.
	record, err := store.Client.OfflineDownload.Create().
		SetHash("fixture-hash").SetAccountID(source.AccountID).SetDirectoryID(source.Directory.ID).
		SetCode("ABP-001").SetJavdbID(knownJavDBID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	service := &Service{database: store.Client, local: local}
	identities := []MovieIdentity{{ID: knownJavDBID, Code: "ABP-001"}}

	// 1. In saving state
	states, err := service.MovieStates(ctx, identities)
	if err != nil || len(states) != 1 || states[0].State != MovieSaving || states[0].LibraryID != 0 {
		t.Fatalf("expected MovieSaving, got %+v (err=%v)", states, err)
	}

	// 2. Offline task completed (status done) awaiting location/scan -> MovieProcessing
	store.Client.OfflineDownload.UpdateOne(record).
		SetStatus(offlinedownload.StatusDone).SetFileID("download-folder").SetAwaitingLocation(true).
		ExecX(ctx)

	states, err = service.MovieStates(ctx, identities)
	if err != nil || len(states) != 1 || states[0].State != MovieProcessing || states[0].LibraryID != 0 {
		t.Fatalf("expected MovieProcessing, got %+v (err=%v)", states, err)
	}

	// 3. Scan task queued -> MovieProcessing
	scanTaskInput := map[string]any{
		"javdb_id": knownJavDBID,
		"source": map[string]any{
			"account_id": source.AccountID,
			"directory":  map[string]any{"id": source.Directory.ID},
		},
	}
	store.Client.Task.Create().
		SetType(tasks.KindScan.String()).
		SetStatus(task.StatusQueued).
		SetPayload(taskPayloadJSON(t, scanTaskInput)).
		SaveX(ctx)

	states, err = service.MovieStates(ctx, identities)
	if err != nil || len(states) != 1 || states[0].State != MovieProcessing || states[0].LibraryID != 0 {
		t.Fatalf("expected MovieProcessing with scan task, got %+v (err=%v)", states, err)
	}

	// 4. Indexed in local library -> MovieInLibrary
	local.movies = []domain.LocalMovie{{ID: 99, Code: "ABP-001", JavDBID: &knownJavDBID}}
	states, err = service.MovieStates(ctx, identities)
	if err != nil || len(states) != 1 || states[0].State != MovieInLibrary || states[0].LibraryID != 99 {
		t.Fatalf("expected MovieInLibrary with ID 99, got %+v (err=%v)", states, err)
	}

	// 5. Unmounted source -> MovieNotInLibrary
	local.source = nil
	states, err = service.MovieStates(ctx, identities)
	if err != nil || len(states) != 1 || states[0].State != MovieNotInLibrary || states[0].LibraryID != 0 {
		t.Fatalf("expected MovieNotInLibrary when unmounted, got %+v (err=%v)", states, err)
	}
}

func TestMovieStatesDifferentAccountOrDirectoryIgnored(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()

	source := domain.LibrarySource{
		AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	local := &stubLocalState{source: &source}
	service := &Service{database: store.Client, local: local}

	store.Client.OfflineDownload.Create().SetHash("other-account-hash").
		SetAccountID("other-account").SetDirectoryID(source.Directory.ID).
		SetJavdbID("other-acc-movie").SetCode("ABP-101").SaveX(ctx)
	store.Client.OfflineDownload.Create().SetHash("other-directory-hash").
		SetAccountID(source.AccountID).SetDirectoryID("other-dir").
		SetJavdbID("other-dir-movie").SetCode("ABP-102").SaveX(ctx)

	states, err := service.MovieStates(ctx, []MovieIdentity{
		{ID: "other-acc-movie", Code: "ABP-101"},
		{ID: "other-dir-movie", Code: "ABP-102"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range states {
		if s.State != MovieNotInLibrary {
			t.Fatalf("item %d crossed boundaries: %+v", i, s)
		}
	}
}
