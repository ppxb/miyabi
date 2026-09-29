package scan

import (
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestFindDirectoryNFOMatchesCatalogueAlias(t *testing.T) {
	files := []pan.File{
		{ID: "different", Name: "LUXU-1099.nfo"},
		{ID: "alias", Name: "259LUXU-1899.nfo"},
	}
	entry, found := findNFO("LUXU-1899", true, files)
	if !found || entry.ID != "alias" {
		t.Fatalf("matching alias NFO was not found: %#v, %v", entry, found)
	}
}

func TestDirectoryNFOSelectionPrefersExactFileOverAliasesAndFolders(t *testing.T) {
	files := []pan.File{
		{ID: "folder", Name: "LUXU-1899.nfo", IsDirectory: true},
		{ID: "alias", Name: "259LUXU-1899.nfo"},
		{ID: "exact", Name: "luxu-1899.NFO"},
	}
	entry, found := findNFO("LUXU-1899", true, files)
	if !found || entry.ID != "exact" {
		t.Fatalf("NFO selection = %+v, found=%t", entry, found)
	}
}
