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
// shared domain and infrastructure packages must not import any of them.
func TestBusinessPackagesDoNotImportEachOther(t *testing.T) {
	const module = "github.com/ppxb/miyabi/internal/"
	business := []string{"catalogue", "library", "library/scan", "library/scrape", "offline", "monitor", "strm", "maintenance", "emby", "subtitle", "network"}
	shared := []string{"drive", "tasks", "database", "domain", "domain/subtitle", "domain/download", "export", "netx"}
	// Subpackages of one bounded context may share code downward only.
	allowed := map[string][]string{
		"library":      {"library/scan"},
		"library/scan": {"library/scrape"},
	}
	forbidden := func(pkg string) []string {
		var list []string
		for _, other := range business {
			if other == pkg {
				continue
			}
			if contains(allowed[pkg], other) {
				continue
			}
			list = append(list, module+other)
		}
		return list
	}
	for _, pkg := range append(append([]string{}, business...), shared...) {
		banned := forbidden(pkg)
		switch pkg {
		case "tasks":
			banned = append(banned, module+"domain")
		case "database":
			banned = append(banned, module+"tasks", module+"drive")
		case "domain", "domain/subtitle", "domain/download":
			banned = append(banned, module+"tasks", module+"drive", module+"database", module+"export")
		}
		for _, imported := range packageImports(t, filepath.Join("..", filepath.FromSlash(pkg))) {
			if contains(allowed[pkg], strings.TrimPrefix(imported, module)) {
				continue
			}
			for _, ban := range banned {
				if imported == ban || strings.HasPrefix(imported, ban+"/") {
					t.Errorf("%s imports %s; dependency must point toward shared types and infrastructure", pkg, imported)
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
