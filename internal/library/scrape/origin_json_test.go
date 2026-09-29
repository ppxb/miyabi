package scrape

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/pan"
)

func TestCoverPayloadPreservesStoredArtworkFileFormat(t *testing.T) {
	const stored = `{"origin":{"nfo":{"id":"1","parent_id":"10","name":"movie.nfo","is_directory":false,"size":20,"pick_code":"nfo-pick","sha1":"nfo-sha"},"poster":{"id":"2","parent_id":"10","name":"poster.jpg","is_directory":false,"size":30,"pick_code":"poster-pick","sha1":"poster-sha"},"fanart":{"id":"3","parent_id":"10","name":"fanart.jpg","is_directory":false,"size":40,"pick_code":"fanart-pick","sha1":"fanart-sha"}}}`
	var payload CoverPayload
	if err := json.Unmarshal([]byte(stored), &payload); err != nil {
		t.Fatal(err)
	}
	want := ArtworkOrigin{
		NFO:    pan.File{ID: "1", ParentID: "10", Name: "movie.nfo", Size: 20, PickCode: "nfo-pick", SHA1: "nfo-sha"},
		Poster: pan.File{ID: "2", ParentID: "10", Name: "poster.jpg", Size: 30, PickCode: "poster-pick", SHA1: "poster-sha"},
		Fanart: pan.File{ID: "3", ParentID: "10", Name: "fanart.jpg", Size: 40, PickCode: "fanart-pick", SHA1: "fanart-sha"},
	}
	if payload.Origin == nil || *payload.Origin != want {
		t.Fatalf("restored origin = %+v", payload.Origin)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]any
	if err := json.Unmarshal([]byte(stored), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before["origin"], after["origin"]) {
		t.Fatalf("persisted origin changed: %s", encoded)
	}
}
