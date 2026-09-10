package javdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeEnvelope(t *testing.T) {
	var result struct {
		Value string `json:"value"`
	}
	if err := decodeEnvelope([]byte(`{"success":1,"data":{"value":"ok"}}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Value != "ok" {
		t.Fatalf("value = %q, want ok", result.Value)
	}
}

func TestWireRatingDecodesNumbersAndDecimalStrings(t *testing.T) {
	for _, test := range []struct {
		input string
		want  float64
	}{
		{`4.5`, 4.5},
		{`"4.5"`, 4.5},
		{`"0.0"`, 0},
		{`null`, 0},
	} {
		t.Run(test.input, func(t *testing.T) {
			var rating wireRating
			if err := json.Unmarshal([]byte(test.input), &rating); err != nil {
				t.Fatal(err)
			}
			if float64(rating) != test.want {
				t.Fatalf("rating = %v, want %v", rating, test.want)
			}
		})
	}
}

func TestWireRatingRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{`""`, `"invalid"`, `true`, `{}`, `[]`} {
		var rating wireRating
		if err := json.Unmarshal([]byte(input), &rating); err == nil {
			t.Errorf("accepted invalid rating %s", input)
		}
	}
}

func TestMovieActorGenderMapping(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{`0`, "female"},
		{`1`, "male"},
		{`null`, "unknown"},
		{`9`, "unknown"},
		{`-1`, "unknown"},
	} {
		var actor wireActor
		if err := json.Unmarshal([]byte(`{"gender":`+test.input+`}`), &actor); err != nil {
			t.Fatal(err)
		}
		movie, err := movieFromWire(wireMovie{ID: "movie", Number: "ABP-001", Actors: []wireActor{actor}})
		if err != nil {
			t.Fatal(err)
		}
		if movie.Actors[0].Gender != test.want {
			t.Errorf("gender %s = %s, want %s", test.input, movie.Actors[0].Gender, test.want)
		}
	}
}

func TestMovieMappingsRejectMissingIdentity(t *testing.T) {
	for _, source := range []wireMovie{
		{Number: "ABP-001"},
		{ID: " \t\n", Number: "ABP-001"},
		{ID: "invalid"},
		{ID: "invalid", Number: " \t\n"},
	} {
		if _, err := movieFromWire(source); err == nil {
			t.Errorf("movie accepted missing identity: %#v", source)
		}
		if _, err := moviesFromWire([]wireMovie{{ID: "valid", Number: "SSIS-589"}, source}); err == nil {
			t.Errorf("movie list accepted missing identity: %#v", source)
		}
	}
}

func TestMovieMappingsPreserveUnfamiliarNumbers(t *testing.T) {
	for _, number := range []string{"KNB-M014", "配信/作品 #0007", "Studio.SpecialEdition", "NO-SEQUENCE", "SCUTE-1575-ITSUKI.mp4"} {
		t.Run(number, func(t *testing.T) {
			movies, err := moviesFromWire([]wireMovie{
				{ID: "main", Number: "GLOD-0436"},
				{ID: "unfamiliar", Number: number},
			})
			if err != nil || len(movies) != 2 || movies[1].Code != number {
				t.Fatalf("unfamiliar number changed or blocked the page: %#v, %v", movies, err)
			}
			references := movieReferencesFromWire("movie", "actor_movies", []wireMovieReference{
				{ID: "main", Number: "GLOD-0436"},
				{ID: "unfamiliar", Number: number},
			})
			if len(references) != 2 || references[1].Code != number {
				t.Fatalf("unfamiliar reference changed or blocked the detail: %#v", references)
			}
		})
	}
}

func TestMovieDetailSkipsInvalidRecommendationsAndReportsOptionalFields(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/main|zh-TW": []byte(`{"success":1,"data":{"movie":{
			"id":"main","number":"ABP-001","title":"Fixture title","cover_url":"https://media.example/cover.jpg","type":9,
			"actors":[{"id":"actor-1","name":"Fixture actor","gender":9}],
			"actor_movies":[
				{"id":"first","number":" ABP-002 ","thumb_url":"https://media.example/first.jpg"},
				{"id":"missing-number"},
				{"number":"ABP-003"},
				{"id":"blank-number","number":" \t\n"},
				{"id":" \t\n","number":"ABP-004"},
				{"id":"last","number":"KNB-M014"}
			],
			"relative_movies":[null,{}, {"id":"related","number":"ABP-005"}]
		}}}`),
	}}
	detail, err := clientWithTransport(transport).MovieDetail(t.Context(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != "main" || detail.Code != "ABP-001" || detail.Title != "Fixture title" ||
		detail.Cover != "https://media.example/cover.jpg" || detail.Zone != ZoneUnknown {
		t.Fatalf("optional metadata damaged the main detail: %#v", detail)
	}
	if len(detail.Actors) != 1 || detail.Actors[0].ID != "actor-1" ||
		detail.Actors[0].Name != "Fixture actor" || detail.Actors[0].Gender != "unknown" {
		t.Fatalf("unknown gender damaged the actor: %#v", detail.Actors)
	}
	wantActorMovies := []MovieReference{
		{ID: "first", Code: "ABP-002", Thumbnail: "https://media.example/first.jpg"},
		{ID: "last", Code: "KNB-M014"},
	}
	if !reflect.DeepEqual(detail.ActorMovies, wantActorMovies) ||
		!reflect.DeepEqual(detail.RelatedMovies, []MovieReference{{ID: "related", Code: "ABP-005"}}) {
		t.Fatalf("valid recommendations were lost or reordered: %#v, %#v", detail.ActorMovies, detail.RelatedMovies)
	}
	fields := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record struct {
			Level   string `json:"level"`
			MovieID string `json:"movie_id"`
			Field   string `json:"field"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Level != "WARN" || record.MovieID != "main" {
			t.Fatalf("diagnostic cannot be traced to the movie: %s", line)
		}
		fields[record.Field]++
	}
	if !reflect.DeepEqual(fields, map[string]int{"type": 1, "actors.gender": 1, "actor_movies": 4, "relative_movies": 2}) {
		t.Fatalf("missing field diagnostics: %#v", fields)
	}
}

func TestMovieDetailReturnsEmptyRecommendationArrays(t *testing.T) {
	for _, recommendations := range []string{"", `,"actor_movies":null,"relative_movies":null`,
		`,"actor_movies":[{}],"relative_movies":[{"id":"missing-number"}]`} {
		transport := &fixtureTransport{responses: map[string][]byte{
			"/api/v4/movies/main|zh-TW": []byte(`{"success":1,"data":{"movie":{"id":"main","number":"ABP-001","type":0` + recommendations + `}}}`),
		}}
		detail, err := clientWithTransport(transport).MovieDetail(t.Context(), "main")
		if err != nil || detail.ActorMovies == nil || detail.RelatedMovies == nil ||
			len(detail.ActorMovies) != 0 || len(detail.RelatedMovies) != 0 {
			t.Fatalf("empty recommendations = %#v, error = %v", detail, err)
		}
	}
}

func TestMovieDetailMapsMissingAndUnknownTypesWithoutGuessing(t *testing.T) {
	for _, test := range []struct {
		field string
		want  Zone
	}{
		{"", ZoneUnknown},
		{`,"type":null`, ZoneUnknown},
		{`,"type":9`, ZoneUnknown},
		{`,"type":-1`, ZoneUnknown},
		{`,"type":0`, ZoneCensored},
		{`,"type":1`, ZoneUncensored},
		{`,"type":2`, ZoneWestern},
		{`,"type":3`, ZoneFC2},
		{`,"type":4`, ZoneAnime},
	} {
		t.Run(test.field, func(t *testing.T) {
			transport := &fixtureTransport{responses: map[string][]byte{
				"/api/v4/movies/main|zh-TW": []byte(`{"success":1,"data":{"movie":{"id":"main","number":"ABP-001"` + test.field + `}}}`),
			}}
			detail, err := clientWithTransport(transport).MovieDetail(t.Context(), "main")
			if err != nil || detail.Zone != test.want || detail.ID != "main" {
				t.Fatalf("detail = %#v, error = %v; want zone %s", detail, err, test.want)
			}
		})
	}
}

func TestMovieDetailStillRejectsIncorrectJSONTypes(t *testing.T) {
	for _, field := range []string{
		`"id":42`, `"number":123`, `"type":"0"`, `"type":true`, `"type":{}`, `"type":1.5`,
		`"actors":[{"gender":"0"}]`, `"actors":[{"gender":false}]`, `"actors":[{"gender":{}}]`,
		`"release_date":20260910`, `"actor_movies":{}`, `"relative_movies":[{"id":42}]`,
	} {
		t.Run(field, func(t *testing.T) {
			var movie map[string]json.RawMessage
			if err := json.Unmarshal([]byte(`{`+field+`}`), &movie); err != nil {
				t.Fatal(err)
			}
			if _, exists := movie["id"]; !exists {
				movie["id"] = json.RawMessage(`"main"`)
			}
			if _, exists := movie["number"]; !exists {
				movie["number"] = json.RawMessage(`"ABP-001"`)
			}
			body, err := json.Marshal(map[string]any{"success": 1, "data": map[string]any{"movie": movie}})
			if err != nil {
				t.Fatal(err)
			}
			transport := &fixtureTransport{responses: map[string][]byte{"/api/v4/movies/main|zh-TW": body}}
			_, err = clientWithTransport(transport).MovieDetail(t.Context(), "main")
			var typeError *json.UnmarshalTypeError
			if !errors.As(err, &typeError) {
				t.Fatalf("incorrect JSON type was swallowed: %v", err)
			}
		})
	}
}

func TestSearchAndResolutionRetainMoviesWithUnknownGender(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[
			{"id":"first","number":"ABP-001","actors":[{"id":"actor-1","gender":9}]},
			{"id":"second","number":"ABP-002","actors":[{"id":"actor-2"}]}
		]}}`),
	}}
	client := clientWithTransport(transport)
	movies, err := client.Search(t.Context(), "ABP", SearchOptions{})
	if err != nil || len(movies) != 2 || movies[0].ID != "first" || movies[1].ID != "second" ||
		movies[0].Actors[0].Gender != "unknown" || movies[1].Actors[0].Gender != "unknown" {
		t.Fatalf("unknown gender damaged the search page: %#v, %v", movies, err)
	}
	if id, err := client.ResolveMovieID(t.Context(), "ABP-001"); err != nil || id != "first" {
		t.Fatalf("unknown gender prevented exact resolution: %q, %v", id, err)
	}
	for _, other := range []string{
		`{"number":"ABP-001"}`,
		`{"id":"missing-number"}`,
		`{"id":"duplicate","number":"ABP-001","actors":[{"gender":9}]}`,
	} {
		transport.responses["/api/v2/search|zh-TW"] = []byte(`{"success":1,"data":{"movies":[{"id":"exact","number":"ABP-001"},` + other + `]}}`)
		if id, err := client.ResolveMovieID(t.Context(), "ABP-001"); err == nil || id != "" {
			t.Fatalf("incomplete or ambiguous search resolved to %q: %v", id, err)
		}
	}
}

func TestMovieDetailPreservesUnknownReferences(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/main|zh-TW": []byte(`{"success":1,"data":{"movie":{
			"id":"main","number":"GLOD-0436","type":1,
			"actor_movies":[{"id":"letter-serial","number":"KNB-M014"}],
			"relative_movies":[{"id":"unfamiliar","number":"作品/限定 #007"}]
		}}}`),
	}}
	client := clientWithTransport(transport)
	detail, err := client.MovieDetail(t.Context(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Code != "GLOD-0436" || len(detail.ActorMovies) != 1 || len(detail.RelatedMovies) != 1 ||
		detail.ActorMovies[0].Code != "KNB-M014" || detail.RelatedMovies[0].Code != "作品/限定 #007" {
		t.Fatalf("detail or references were changed: %#v", detail)
	}
}

func TestResolveMovieIDKeepsCompleteUnfamiliarNumbers(t *testing.T) {
	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v2/search|zh-TW": []byte(`{"success":1,"data":{"movies":[
			{"id":"partial","number":"M-014"},
			{"id":"neighbor","number":"KNB-M015"},
			{"id":"exact","number":"knb-m014"},
			{"id":"unfamiliar","number":"作品/限定 #007"}
		]}}`),
	}}
	client := clientWithTransport(transport)
	for number, want := range map[string]string{"KNB_M014": "exact", "M-014": "partial", "作品/限定 #007": "unfamiliar"} {
		id, err := client.ResolveMovieID(t.Context(), number)
		if err != nil || id != want {
			t.Fatalf("resolve %q = %q, %v; want %q", number, id, err, want)
		}
	}
	if id, err := client.ResolveMovieID(t.Context(), "KNB-M01"); err == nil {
		t.Fatalf("accepted partial result %q", id)
	}
}

func TestDecodeEnvelopeAPIError(t *testing.T) {
	err := decodeEnvelope([]byte(`{"success":0,"action":"BadRequest","message":"invalid"}`), nil)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiError.Action != "BadRequest" || apiError.Message != "invalid" {
		t.Fatalf("APIError = %#v", apiError)
	}
}

func TestDecodeEnvelopeRejectsTrailingJSON(t *testing.T) {
	err := decodeEnvelope([]byte(`{"success":1,"data":{}} trailing`), &struct{}{})
	if err == nil {
		t.Fatal("decodeEnvelope accepted trailing JSON")
	}
}
