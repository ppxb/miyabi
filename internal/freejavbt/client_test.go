package freejavbt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"golang.org/x/time/rate"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func newTestClient(body string) *Client {
	client := &Client{
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})},
		base:    defaultBaseURL,
		limiter: rate.NewLimiter(rate.Inf, 0),
	}
	return client
}

func TestFetchMetadata(t *testing.T) {
	client := newTestClient(sampleHTML)
	defer client.Close()

	result, err := client.Fetch(context.Background(), domain.MovieRef{Code: "SONE-001"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Detail.Title != "SONE-001 テスト" || len(result.Detail.Actors) != 2 || result.Detail.ReleaseDate != "2024-01-02" {
		t.Fatalf("metadata = %+v", result.Detail.Movie)
	}
	if len(result.Images) != 3 {
		t.Fatalf("images = %d", len(result.Images))
	}
}

func TestFindMagnets(t *testing.T) {
	client := newTestClient(sampleHTML)
	defer client.Close()

	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: "SONE-001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(magnets) != 1 || magnets[0].Sources[0] != domain.MagnetSourceFreeJavBT || !magnets[0].HasSubtitle {
		t.Fatalf("magnets = %+v", magnets)
	}
}

func TestFindSkipsEmptyCode(t *testing.T) {
	client := newTestClient(sampleHTML)
	defer client.Close()
	magnets, err := client.Find(context.Background(), domain.MovieRef{Code: ""})
	if err != nil || len(magnets) != 0 {
		t.Fatalf("magnets = %v err = %v", magnets, err)
	}
}
