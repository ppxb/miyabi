package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type movieStateStub struct {
	Discoverer
	movies []service.MovieIdentity
}

func (stub *movieStateStub) MovieStates(_ context.Context, movies []service.MovieIdentity) ([]service.DiscoverMovieState, error) {
	stub.movies = movies
	return []service.DiscoverMovieState{{ID: movies[0].ID, State: service.MovieInLibrary, LibraryID: 42}}, nil
}

func TestMovieStatesHandlerValidatesBatchesAndDisablesHTTPCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := service.MovieIdentity{ID: "catalogue-id", Code: "ABP-001"}
	tooMany := make([]service.MovieIdentity, 101)
	for index := range tooMany {
		tooMany[index] = valid
	}
	for _, scenario := range []struct {
		name   string
		movies []service.MovieIdentity
		valid  bool
	}{
		{"valid batch", []service.MovieIdentity{valid}, true},
		{"empty batch", []service.MovieIdentity{}, false},
		{"missing id", []service.MovieIdentity{{Code: valid.Code}}, false},
		{"missing code", []service.MovieIdentity{{ID: valid.ID}}, false},
		{"oversized batch", tooMany, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			body, err := json.Marshal(movieStatesInput{Movies: scenario.movies})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/discover/movie-states", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			stub := &movieStateStub{}
			discoverMovieStatesHandler(stub)(c)
			if !scenario.valid {
				if len(c.Errors) != 1 || stub.movies != nil {
					t.Fatalf("invalid batch reached the service: errors=%v movies=%v", c.Errors, stub.movies)
				}
				return
			}
			var states []service.DiscoverMovieState
			if err := json.Unmarshal(response.Body.Bytes(), &states); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
				len(states) != 1 || states[0].ID != valid.ID || states[0].LibraryID != 42 ||
				len(stub.movies) != 1 || stub.movies[0] != valid {
				t.Fatalf("incorrect state response: %s, movies=%v", response.Body, stub.movies)
			}
		})
	}
}
