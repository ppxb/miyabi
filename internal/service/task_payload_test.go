package service

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/nfo"
)

func taskPayloadJSON(t testing.TB, value any) json.RawMessage {
	t.Helper()
	payload, err := encodeTaskPayload(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestTaskPayloadRoundTripKeepsMetadataAndIntegerPrecision(t *testing.T) {
	library, _, payload := libraryFixture(t)
	input := coverPayload{
		metadataPayload: metadataPayload{Source: payload.Source, ScanTaskID: 9007199254740993, MovieID: 2, Code: "ABP-001"},
		Document: nfo.Movie{Code: "ABP-001", Title: "Fixture title", Rating: 4.5,
			Tags: []nfo.Tag{{ID: "tag", Name: "标签", CategoryID: "category"}}},
		Snapshot: &metadataSnapshot{Videos: "fingerprint", Directories: []metadataDirectorySnapshot{{ID: "10"}}},
	}
	encoded := taskPayloadJSON(t, input)
	record := library.database.Task.Create().SetType("cover").SetPayload(encoded).SaveX(t.Context())
	loaded := library.database.Task.GetX(t.Context(), record.ID)
	restored, err := decodeTaskPayload[coverPayload](loaded.Payload)
	if err != nil || !reflect.DeepEqual(restored, input) {
		t.Fatalf("task round trip changed metadata or IDs: %#v, %v", restored, err)
	}
	if len(loaded.Payload) == 0 || loaded.Payload[0] != '{' {
		t.Fatalf("task stored a JSON string instead of an object: %s", loaded.Payload)
	}
	if count := library.database.Task.Query().Where(task.TypeEQ("cover")).CountX(t.Context()); count != 1 {
		t.Fatalf("round trip changed task type: %d", count)
	}
}

func TestPartialTaskPayloadUpdateRetainsUnknownFields(t *testing.T) {
	original := json.RawMessage(`{"hash":"fixture","scan_task_id":9007199254740993,"future":{"nested":[true,1,"text"]}}`)
	updated, err := setTaskPayloadField(original, "file_ids", []string{"video"})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Hash   string          `json:"hash"`
		ID     int64           `json:"scan_task_id"`
		Files  []string        `json:"file_ids"`
		Future json.RawMessage `json:"future"`
	}
	if err := json.Unmarshal(updated, &result); err != nil {
		t.Fatal(err)
	}
	if result.Hash != "fixture" || result.ID != 9007199254740993 ||
		!reflect.DeepEqual(result.Files, []string{"video"}) || string(result.Future) != `{"nested":[true,1,"text"]}` {
		t.Fatalf("partial update changed unrelated task data: %s", updated)
	}
	if string(original) != `{"hash":"fixture","scan_task_id":9007199254740993,"future":{"nested":[true,1,"text"]}}` {
		t.Fatal("partial update mutated the original task payload")
	}
}

func TestTaskPayloadRejectsMalformedValues(t *testing.T) {
	for _, body := range []string{`{`, `{"scan_task_id":"1"}`, `{"scan_task_id":1.5}`, `{"scan_task_id":9223372036854775808}`} {
		if _, err := decodeTaskPayload[metadataPayload](json.RawMessage(body)); err == nil {
			t.Errorf("accepted invalid task payload: %s", body)
		}
	}
	if _, err := encodeTaskPayload(nfo.Movie{Rating: math.NaN()}); err == nil {
		t.Fatal("accepted an unencodable task")
	}
	for _, body := range []string{`null`, `[]`, `"text"`} {
		if _, err := setTaskPayloadField(json.RawMessage(body), "file_ids", []string{"video"}); err == nil {
			t.Errorf("patched a non-object task payload: %s", body)
		}
	}
}
