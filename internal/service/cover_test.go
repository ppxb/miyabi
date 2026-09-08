package service

import (
	"testing"

	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/pan"
)

func TestCoverRetryPreservesEditsAndRecognizesItsOwnWriteback(t *testing.T) {
	poster, fanart := []byte("fixture poster"), []byte("fixture fanart")
	origin := artworkOrigin{
		Poster: pan.File{ParentID: "10", Name: "poster.jpg", SHA1: pan.SHA1(poster)},
		Fanart: pan.File{ParentID: "10", Name: "fanart.jpg", SHA1: pan.SHA1(fanart)},
	}
	doc := nfo.Movie{Code: "ABP-001", Title: "Fixture title"}
	current := doc
	current.Thumbs = []nfo.Thumb{{Aspect: "poster", Path: "poster.jpg"}}
	current.Fanart = "fanart.jpg"
	if err := verifyCoverOrigin(coverPayload{Document: doc}, "10", current, origin, poster, fanart); err != nil {
		t.Fatalf("own writeback was rejected: %v", err)
	}
	input := coverPayload{Document: current, Origin: &origin}
	if err := verifyCoverOrigin(input, "10", current, origin, poster, fanart); err != nil {
		t.Fatalf("unchanged NFO was rejected: %v", err)
	}
	edited := current
	edited.Title = "User edit"
	if err := verifyCoverOrigin(input, "10", edited, origin, poster, fanart); err == nil {
		t.Fatal("edited NFO would be marked synchronized with old metadata")
	}
	changed := origin
	changed.Poster.SHA1 = pan.SHA1([]byte("edited poster"))
	if err := verifyCoverOrigin(input, "10", current, changed, poster, fanart); err == nil {
		t.Fatal("edited artwork would be marked synchronized with old cache")
	}
	if err := verifyCoverOrigin(coverPayload{Document: doc}, "10", edited, origin, poster, fanart); err == nil {
		t.Fatal("new user NFO was mistaken for an interrupted writeback")
	}
}
