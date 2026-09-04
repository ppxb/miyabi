package javdb

import (
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
