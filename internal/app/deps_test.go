package app_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Business packages may only reach each other through domain types or
// interfaces declared by the caller. This test pins the import direction:
// a business package must not import another business package, and the
// shared kernel (drive, tasks, database) must not import any of them.
func TestBusinessPackagesDoNotImportEachOther(t *testing.T) {
	const module = "github.com/ppxb/miyabi/internal/"
	business := []string{"catalogue", "library", "library/scan", "library/scrape", "offline", "monitor", "strm", "maintenance"}
	kernel := []string{"drive", "tasks", "database"}
	// Subpackages of one bounded context may share code downward only.
	allowed := map[string][]string{
		"library":      {"library/scan"},
		"library/scan": {"library/scrape"},
	}
	forbidden := func(pkg string) []string {
		var list []string
		for _, other := range business {
			if other == pkg || strings.HasPrefix(other, pkg+"/") && contains(allowed[pkg], other) {
				continue
			}
			if contains(allowed[pkg], other) {
				continue
			}
			list = append(list, module+other)
		}
		return list
	}
	for _, pkg := range append(append([]string{}, business...), kernel...) {
		banned := forbidden(pkg)
		if contains(kernel, pkg) {
			banned = nil
			for _, other := range business {
				banned = append(banned, module+other)
			}
		}
		for _, imported := range packageImports(t, filepath.Join("..", filepath.FromSlash(pkg))) {
			for _, ban := range banned {
				if imported == ban {
					t.Errorf("%s imports %s; business packages must talk through domain types or caller-defined interfaces", pkg, imported)
				}
			}
		}
	}
}

func packageImports(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var imports []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			if !seen[path] {
				seen[path] = true
				imports = append(imports, path)
			}
		}
	}
	return imports
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
