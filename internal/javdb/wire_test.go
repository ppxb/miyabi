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
