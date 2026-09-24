package gfriends

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGFriendsClient_LookupAndCache(t *testing.T) {
	tempDir := t.TempDir()

	tree := FileTree{
		Content: map[string]map[string]string{
			"S": {
				"三上悠亜.jpg":    "三上悠亜.jpg?t=12345",
				"Yua Mikami.jpg": "三上悠亜.jpg?t=12345",
			},
			"A": {
				"相沢みなみ.jpg": "相沢みなみ.jpg?t=67890",
			},
		},
	}
	treeData, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}

	imageBytes := []byte("fake-jpeg-data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Filetree.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(treeData)
			return
		}
		if r.URL.Path == "/Content/S/三上悠亜.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(imageBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Write pre-existing cache file to test loading from cache
	cacheFile := filepath.Join(tempDir, "gfriends_tree.json")
	if err := os.WriteFile(cacheFile, treeData, 0644); err != nil {
		t.Fatal(err)
	}

	client := New(tempDir, server.Client())
	if err := client.EnsureIndex(t.Context()); err != nil {
		t.Fatalf("EnsureIndex failed: %v", err)
	}

	// Test lookups
	cases := []struct {
		name    string
		wantRel string
		found   bool
	}{
		{"三上悠亜", "Content/S/三上悠亜.jpg?t=12345", true},
		{"Yua Mikami", "Content/S/三上悠亜.jpg?t=12345", true},
		{"yua mikami", "Content/S/三上悠亜.jpg?t=12345", true},
		{"相沢みなみ", "Content/A/相沢みなみ.jpg?t=67890", true},
		{"未知演员", "", false},
	}

	for _, tc := range cases {
		rel, ok := client.Lookup(tc.name)
		if ok != tc.found {
			t.Errorf("Lookup(%q) found=%v, want %v", tc.name, ok, tc.found)
		}
		if ok && rel != tc.wantRel {
			t.Errorf("Lookup(%q) = %q, want %q", tc.name, rel, tc.wantRel)
		}
	}
}

func TestGFriendsClient_PrefersFirstFolderDeterministically(t *testing.T) {
	tree := FileTree{Content: map[string]map[string]string{
		"2-Other":  {"三上悠亜.jpg": "三上悠亜.jpg?t=2"},
		"0-Manual": {"三上悠亜.jpg": "三上悠亜.jpg?t=0"},
		"1-Studio": {"三上 悠亜.jpg": "三上悠亜.jpg?t=1"},
	}}
	for range 20 {
		client := New("", nil)
		client.buildIndexLocked(tree)
		if rel, _ := client.Lookup("三上悠亜"); rel != "Content/0-Manual/三上悠亜.jpg?t=0" {
			t.Fatalf("Lookup chose %q", rel)
		}
	}
}
