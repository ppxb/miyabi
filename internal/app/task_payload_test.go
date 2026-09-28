package app

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

func taskPayloadJSON(t testing.TB, value any) json.RawMessage {
	t.Helper()
	payload, err := tasks.EncodePayload(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestTaskPayloadRoundTripKeepsMetadataAndIntegerPrecision(t *testing.T) {
	fix := libraryFixture(t)
	input := scrape.CoverPayload{
		MetadataPayload: scrape.MetadataPayload{Source: fix.Payload.Source, ScanTaskID: 9007199254740993, MovieID: 2, Code: "ABP-001"},
		Document: nfo.Movie{Code: "ABP-001", Title: "Fixture title", Rating: 4.5,
			Tags: []nfo.Tag{{ID: "tag", Name: "标签", CategoryID: "category"}}},
		Completed: true,
	}
	encoded := taskPayloadJSON(t, input)
	record := fix.DB.Task.Create().SetType("cover").SetPayload(encoded).SaveX(t.Context())
	loaded := fix.DB.Task.GetX(t.Context(), record.ID)
	restored, err := tasks.DecodePayload[scrape.CoverPayload](loaded.Payload)
	if err != nil || !reflect.DeepEqual(restored, input) {
		t.Fatalf("task round trip changed metadata or IDs: %#v, %v", restored, err)
	}
	if len(loaded.Payload) == 0 || loaded.Payload[0] != '{' {
		t.Fatalf("task stored a JSON string instead of an object: %s", loaded.Payload)
	}
	if count := fix.DB.Task.Query().Where(task.TypeEQ("cover")).CountX(t.Context()); count != 1 {
		t.Fatalf("round trip changed task type: %d", count)
	}
}

func TestPartialTaskPayloadUpdateRetainsUnknownFields(t *testing.T) {
	original := json.RawMessage(`{"hash":"fixture","scan_task_id":9007199254740993,"future":{"nested":[true,1,"text"]}}`)
	updated, err := tasks.SetPayloadField(original, "file_ids", []string{"video"})
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
		if _, err := tasks.DecodePayload[scrape.MetadataPayload](json.RawMessage(body)); err == nil {
			t.Errorf("accepted invalid task payload: %s", body)
		}
	}
	if _, err := tasks.EncodePayload(nfo.Movie{Rating: math.NaN()}); err == nil {
		t.Fatal("accepted an unencodable task")
	}
	for _, body := range []string{`null`, `[]`, `"text"`} {
		if _, err := tasks.SetPayloadField(json.RawMessage(body), "file_ids", []string{"video"}); err == nil {
			t.Errorf("patched a non-object task payload: %s", body)
		}
	}
}

var taskBenchmarkResult any

func BenchmarkTaskPayload(b *testing.B) {
	input := scrape.CoverPayload{
		MetadataPayload: scrape.MetadataPayload{Source: domain.LibrarySource{AccountID: "100", Directory: domain.LibraryDirectory{ID: "10", Path: "/Movies"}},
			ScanTaskID: 1, MovieID: 2, Code: "ABP-001", JavDBID: "movie"},
		Document:  nfo.Movie{Code: "ABP-001", Title: "Fixture title", Rating: 4.5},
		Completed: true,
	}
	for i := range 20 {
		input.Document.Tags = append(input.Document.Tags, nfo.Tag{ID: fmt.Sprint(i), Name: "Fixture tag", CategoryID: "category"})
	}
	body, err := json.Marshal(input)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			encoded, err := tasks.EncodePayload(input)
			if err != nil {
				b.Fatal(err)
			}
			stored, err := json.Marshal(encoded)
			if err != nil {
				b.Fatal(err)
			}
			taskBenchmarkResult = stored
		}
	})
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var record ent.Task
			if err := json.Unmarshal(body, &record.Payload); err != nil {
				b.Fatal(err)
			}
			decoded, err := tasks.DecodePayload[scrape.CoverPayload](record.Payload)
			if err != nil {
				b.Fatal(err)
			}
			taskBenchmarkResult = decoded
		}
	})
}
