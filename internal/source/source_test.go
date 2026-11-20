package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.Client)
}

func TestReplaceValidates(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		source Source
	}{
		{"empty name", Source{ID: "1", Kind: KindLocal, Name: " ", Config: map[string]string{"path": dir}}},
		{"unknown kind", Source{ID: "1", Kind: "ftp", Name: "x"}},
		{"local missing path", Source{ID: "1", Kind: KindLocal, Name: "x"}},
		{"local not directory", Source{ID: "1", Kind: KindLocal, Name: "x", Config: map[string]string{"path": file}}},
		{"local missing directory", Source{ID: "1", Kind: KindLocal, Name: "x", Config: map[string]string{"path": filepath.Join(dir, "nope")}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := service.Replace(ctx, []Source{testCase.source}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestReplaceAssignsIDAndPersists(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	dir := t.TempDir()

	saved, err := service.Replace(ctx, []Source{
		{Kind: KindLocal, Name: "Local", Enabled: true, Config: map[string]string{"path": dir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].ID == "" {
		t.Fatal("expected an assigned id")
	}
	list, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != saved[0].ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestLocalRoots(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	first := t.TempDir()
	second := t.TempDir()

	if _, err := service.Replace(ctx, []Source{
		{Kind: KindLocal, Name: "first", Enabled: true, Config: map[string]string{"path": first}},
		{Kind: KindLocal, Name: "second", Enabled: false, Config: map[string]string{"path": second}},
		{Kind: Kind115, Name: "cloud", Enabled: true, Config: map[string]string{"account_id": "A"}},
	}); err != nil {
		t.Fatal(err)
	}

	roots, err := service.LocalRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != first {
		t.Fatalf("roots = %v", roots)
	}
}

type fakeMounts struct {
	mounts map[string]domain.LibrarySource
}

func (f fakeMounts) Mount(_ context.Context, accountID string) (domain.LibrarySource, bool, error) {
	source, ok := f.mounts[accountID]
	return source, ok, nil
}

func TestLibrarySources(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t)
	mounted := domain.LibrarySource{
		AccountID: "A",
		Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	service.SetMountProvider(fakeMounts{mounts: map[string]domain.LibrarySource{"A": mounted}})

	if _, err := service.Replace(ctx, []Source{
		{Kind: Kind115, Name: "bound", Enabled: true, Config: map[string]string{"account_id": "A"}},
		{Kind: Kind115, Name: "disabled", Enabled: false, Config: map[string]string{"account_id": "A"}},
		{Kind: Kind115, Name: "unmounted", Enabled: true, Config: map[string]string{"account_id": "ZZ"}},
	}); err != nil {
		t.Fatal(err)
	}

	list, err := service.LibrarySources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AccountID != "A" || list[0].Directory.ID != "10" {
		t.Fatalf("library sources = %+v", list)
	}
}
