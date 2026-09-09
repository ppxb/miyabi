package javdb

import (
	"encoding/json"
	"errors"
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
	invalid := 9
	if _, err := movieFromWire(wireMovie{ID: "movie", Number: "ABP-001", Actors: []wireActor{{Gender: &invalid}}}); err == nil {
		t.Fatal("accepted an unsupported actor gender")
	}
}

func TestMovieMappingsRejectMissingNumbers(t *testing.T) {
	for _, number := range []string{"", " \t\n"} {
		t.Run(number, func(t *testing.T) {
			if _, err := moviesFromWire([]wireMovie{
				{ID: "valid", Number: "SSIS-589"},
				{ID: "invalid", Number: number},
			}); err == nil {
				t.Errorf("movies accepted missing number %q", number)
			}
			if _, err := movieReferencesFromWire([]wireMovieReference{
				{ID: "valid", Number: "SSIS-589"},
				{ID: "invalid", Number: number},
			}); err == nil {
				t.Errorf("references accepted missing number %q", number)
			}
		})
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
			references, err := movieReferencesFromWire([]wireMovieReference{
				{ID: "main", Number: "GLOD-0436"},
				{ID: "unfamiliar", Number: number},
			})
			if err != nil || len(references) != 2 || references[1].Code != number {
				t.Fatalf("unfamiliar reference changed or blocked the detail: %#v, %v", references, err)
			}
		})
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
