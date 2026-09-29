package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
)

type viewedStub struct {
	ViewedManager
	ids []string
}

func (stub *viewedStub) ViewedMovieIDs(_ context.Context) ([]string, error) {
	return stub.ids, nil
}

func (stub *viewedStub) AddViewedMovieIDs(_ context.Context, ids []string) error {
	stub.ids = append(stub.ids, ids...)
	return nil
}

func TestDiscoverViewedHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("GET /discover/viewed returns list and no-store", func(t *testing.T) {
		stub := &viewedStub{ids: []string{"id-1", "id-2"}}
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/discover/viewed", nil)

		noStore()(c)
		discoverViewedHandler(stub)(c)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
		}
		var got []string
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, stub.ids) {
			t.Fatalf("got %v, want %v", got, stub.ids)
		}
	})

	t.Run("POST /discover/viewed accepts valid batch", func(t *testing.T) {
		stub := &viewedStub{}
		payload := addViewedInput{IDs: []string{"id-a", "id-b"}}
		body, _ := json.Marshal(payload)

		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/discover/viewed", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		discoverAddViewedHandler(stub)(c)

		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if !slices.Equal(stub.ids, payload.IDs) {
			t.Fatalf("stub IDs = %v, want %v", stub.ids, payload.IDs)
		}
	})

	t.Run("POST /discover/viewed rejects empty batch", func(t *testing.T) {
		stub := &viewedStub{}
		payload := addViewedInput{IDs: []string{}}
		body, _ := json.Marshal(payload)

		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/discover/viewed", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")

		discoverAddViewedHandler(stub)(c)

		if len(c.Errors) != 1 {
			t.Fatalf("expected error for empty batch, got errors: %v", c.Errors)
		}
	})
}
