package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func newTestService(t *testing.T, mounts MountProvider) *Service {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return New(store.Client, mounts)
}

func TestReplaceValidates(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t, nil)
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
		{"pikpak not supported yet", Source{ID: "1", Kind: KindPikPak, Name: "x"}},
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
	service := newTestService(t, nil)
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
	service := newTestService(t, nil)
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

func (f fakeMounts) Mounts(context.Context) ([]domain.LibrarySource, error) {
	result := make([]domain.LibrarySource, 0, len(f.mounts))
	for _, source := range f.mounts {
		result = append(result, source)
	}
	return result, nil
}

func TestLibrarySources(t *testing.T) {
	ctx := t.Context()
	mounted := domain.LibrarySource{
		AccountID: "A",
		Directory: domain.LibraryDirectory{ID: "10", Name: "Movies", Path: "/Movies"},
	}
	service := newTestService(t, fakeMounts{mounts: map[string]domain.LibrarySource{"A": mounted}})

	if _, err := service.Replace(ctx, []Source{
		{Kind: Kind115, Name: "broken", Enabled: true, Config: map[string]string{"account_id": "ERR"}},
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

func TestLibrarySourcesSeparatesProviders(t *testing.T) {
	ctx := t.Context()
	mounts := map[string]domain.LibrarySource{
		"115-A": {
			AccountID: "A",
			Directory: domain.LibraryDirectory{ID: "10", Name: "115 media", Path: "/115"},
		},
		"guangya-A": {
			Provider:  string(KindGuangya),
			AccountID: "A",
			Directory: domain.LibraryDirectory{ID: "20", Name: "Guangya media", Path: "/guangya"},
		},
		"guangya-B": {
			Provider:  string(KindGuangya),
			AccountID: "B",
			Directory: domain.LibraryDirectory{ID: "30", Name: "Other media", Path: "/other"},
		},
	}
	service := newTestService(t, fakeMounts{mounts: mounts})
	if _, err := service.Replace(ctx, []Source{
		{Kind: Kind115, Name: "115", Enabled: true, Config: map[string]string{"account_id": "A"}},
		{Kind: KindGuangya, Name: "guangya", Enabled: true, Config: map[string]string{"account_id": "A"}},
		{Kind: KindGuangya, Name: "other", Enabled: true, Config: map[string]string{"account_id": "B"}},
	}); err != nil {
		t.Fatal(err)
	}
	list, err := service.LibrarySources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("library sources = %+v", list)
	}
	byProviderAccount := make(map[string]string, len(list))
	for _, source := range list {
		provider := source.Provider
		if provider == "" {
			provider = "115"
		}
		byProviderAccount[provider+"|"+source.AccountID] = source.Directory.ID
	}
	for key, directory := range map[string]string{
		"115|A":     "10",
		"guangya|A": "20",
		"guangya|B": "30",
	} {
		if byProviderAccount[key] != directory {
			t.Errorf("%s directory = %q, want %q", key, byProviderAccount[key], directory)
		}
	}
}

func TestReplaceKeepsOrderAndClears(t *testing.T) {
	ctx := t.Context()
	service := newTestService(t, nil)
	dir := t.TempDir()
	list := []Source{
		{ID: "b", Kind: KindLocal, Name: "second", Enabled: true, Config: map[string]string{"path": dir}},
		{ID: "a", Kind: KindLocal, Name: "first", Enabled: false, Config: map[string]string{"path": dir}},
	}
	if _, err := service.Replace(ctx, list); err != nil {
		t.Fatal(err)
	}
	got, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" || got[1].Enabled || got[0].Config["path"] != dir {
		t.Fatalf("sources = %+v", got)
	}
	if _, err := service.Replace(ctx, []Source{list[0], list[0]}); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("duplicate ids: err = %v", err)
	}
	if _, err := service.Replace(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := service.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("after clear: %+v, %v", got, err)
	}
}
