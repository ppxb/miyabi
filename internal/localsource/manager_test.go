package localsource

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewManager(store.Client)
}

func canonical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestLocalSourceIdentityFollowsTheCanonicalPath(t *testing.T) {
	manager := newManager(t)
	ctx := t.Context()
	root := t.TempDir()
	nested := filepath.Join(root, "inbox")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	first, err := manager.Add(ctx, "下载目录", root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != canonical(t, root) || first.Name != "下载目录" || !first.Enabled {
		t.Fatalf("added source = %+v", first)
	}
	if err := manager.Remove(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	// A non-canonical spelling of the same directory keeps the identity stable.
	second, err := manager.Add(ctx, "", filepath.Join(nested, ".."))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Path != first.Path {
		t.Fatalf("identity changed with the path spelling: %+v vs %+v", second, first)
	}
	if second.Name != filepath.Base(root) {
		t.Fatalf("empty name was not defaulted: %q", second.Name)
	}
}

func TestLocalSourceRejectsDuplicatesAndInvalidDirectories(t *testing.T) {
	manager := newManager(t)
	ctx := t.Context()
	root := t.TempDir()
	if _, err := manager.Add(ctx, "媒体", root); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"duplicate":  filepath.Join(root, "."),
		"missing":    filepath.Join(root, "absent"),
		"file":       writeFile(t, root),
		"empty path": "",
	} {
		if _, err := manager.Add(ctx, name, path); !domain.IsKind(err, domain.KindConflict) && !domain.IsKind(err, domain.KindInvalid) {
			t.Fatalf("%s: err = %v, want a conflict or invalid error", name, err)
		}
	}
}

func writeFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(path, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLocalSourceUpdateRemoveAndFind(t *testing.T) {
	manager := newManager(t)
	ctx := t.Context()
	source, err := manager.Add(ctx, "媒体", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	disabled, name := false, "离线盘"
	updated, err := manager.Update(ctx, source.ID, Update{Name: &name, Enabled: &disabled})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.Enabled {
		t.Fatalf("updated source = %+v", updated)
	}
	// A disabled source stays configured: the scan refuses it, not the settings.
	found, ok, err := manager.Find(ctx, source.ID)
	if err != nil || !ok || found != updated {
		t.Fatalf("find = %+v, %v, %v", found, ok, err)
	}
	if _, err := manager.Update(ctx, source.ID, Update{Name: new(string)}); !domain.IsKind(err, domain.KindInvalid) {
		t.Fatalf("empty rename err = %v", err)
	}
	if _, err := manager.Update(ctx, "src-missing", Update{}); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("unknown update err = %v", err)
	}

	if err := manager.Remove(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(ctx, source.ID); !domain.IsKind(err, domain.KindNotFound) {
		t.Fatalf("second remove err = %v", err)
	}
	if sources, err := manager.List(ctx); err != nil || len(sources) != 0 {
		t.Fatalf("list after remove = %+v, %v", sources, err)
	}
}
