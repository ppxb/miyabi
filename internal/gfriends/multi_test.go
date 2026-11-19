package gfriends

import "testing"

func TestBuildAllIndexKeepsEveryImage(t *testing.T) {
	tree := fileTree{Content: map[string]map[string]string{
		"A": {"Actor.jpg": "actor-a.jpg"},
		"B": {"Actor.jpg": "actor-b.jpg"},
		"C": {"Other.jpg": "other.jpg"},
	}}
	all := buildAllIndex(tree)
	if len(all["actor"]) != 2 {
		t.Fatalf("expected 2 images for actor, got %d (%v)", len(all["actor"]), all["actor"])
	}
	if len(all["other"]) != 1 {
		t.Fatalf("expected 1 image for other, got %d", len(all["other"]))
	}
}
