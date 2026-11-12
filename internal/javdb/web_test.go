package javdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/netx"
)

var webAuthRequiredEnvelope = []byte(`{"success":0,"action":"JWTVerificationError","message":"請登錄帳號","data":null}`)

func webFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestParseWebMovieDetail(t *testing.T) {
	detail, err := parseWebMovieDetail(webFixture(t, "web_movie.html"), "QD3n9")
	if err != nil {
		t.Fatal(err)
	}
	if detail.ID != "QD3n9" || detail.Code != "IPX-811" || detail.Title != "嗑藥做愛住同房NTR姦  田中柠檬" {
		t.Fatalf("identity = %#v", detail.Movie)
	}
	if detail.ReleaseDate != "2022-02-08" || detail.Duration != 110 || detail.Rating != 4.71 {
		t.Fatalf("stats = %#v", detail.Movie)
	}
	if detail.Cover != "https://c0.jdbstatic.com/covers/qd/QD3n9.jpg" || detail.Thumbnail != detail.Cover {
		t.Fatalf("cover = %q thumbnail = %q", detail.Cover, detail.Thumbnail)
	}
	if detail.Maker == nil || detail.Maker.ID != "ZXX" || detail.Maker.Name != "IDEA POCKET" {
		t.Fatalf("maker = %#v", detail.Maker)
	}
	if detail.Series == nil || detail.Series.ID != "eAb" || detail.Series.Name != "キメセク" {
		t.Fatalf("series = %#v", detail.Series)
	}
	if detail.Director == nil || detail.Director.ID != "65e0" {
		t.Fatalf("director = %#v", detail.Director)
	}
	if len(detail.Actors) != 2 || detail.Actors[0].ID != "kzx6" || detail.Actors[0].Gender != "female" || detail.Actors[1].Gender != "unknown" {
		t.Fatalf("actors = %#v", detail.Actors)
	}
	if len(detail.Tags) != 3 || detail.Tags[0].Name != "戲劇" {
		t.Fatalf("tags = %#v", detail.Tags)
	}
	if len(detail.PreviewImages) != 2 || detail.PreviewImages[0].Original != "https://c0.jdbstatic.com/samples/qd/QD3n9_l_0.jpg" ||
		detail.PreviewImages[0].Thumbnail != "https://c0.jdbstatic.com/samples/qd/QD3n9_s_0.jpg" || !detail.HasPreview {
		t.Fatalf("previews = %#v", detail.PreviewImages)
	}
	if len(detail.ActorMovies) != 2 || detail.ActorMovies[0].ID != "O7MZO" || detail.ActorMovies[0].Code != "IPX-484" {
		t.Fatalf("actor movies = %#v", detail.ActorMovies)
	}
	if len(detail.RelatedMovies) != 1 || detail.RelatedMovies[0].Code != "IPX-018" {
		t.Fatalf("related movies = %#v", detail.RelatedMovies)
	}
}

func TestRequiresWebFallback(t *testing.T) {
	if !requiresWebFallback(&APIError{Action: "JWTVerificationError", Message: "請登錄帳號"}) {
		t.Fatal("JWT verification error must fall back")
	}
	if !requiresWebFallback(&HTTPError{StatusCode: http.StatusForbidden}) {
		t.Fatal("HTTP 403 must fall back")
	}
	if requiresWebFallback(&APIError{Action: "NotFound", Message: "missing"}) {
		t.Fatal("ordinary API error must not fall back")
	}
	if requiresWebFallback(nil) {
		t.Fatal("nil error must not fall back")
	}
}

func TestDetailFallsBackToWebAndSearchRequiresLogin(t *testing.T) {
	movieHTML := webFixture(t, "web_movie.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v/QD3n9" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(movieHTML)
	}))
	defer server.Close()

	previousBase := webBaseURL
	webBaseURL = server.URL
	defer func() { webBaseURL = previousBase }()

	fingerprint, err := netx.NewProxiedFingerprintClient(nil, netx.FingerprintOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer fingerprint.Close()

	transport := &fixtureTransport{responses: map[string][]byte{
		"/api/v4/movies/QD3n9": webAuthRequiredEnvelope,
	}}
	client := clientWithTransport(transport)
	client.SetCredentials("", "")
	client.fingerprint = fingerprint

	// Without an account, search reports a login prompt and never calls the API.
	if _, err := client.Search(context.Background(), "SSNI-497", domain.SearchOptions{Page: 1, Limit: 20}); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("search error = %v, want ErrLoginRequired", err)
	}
	if len(transport.calls) != 0 {
		t.Fatalf("search made %d API calls, want 0", len(transport.calls))
	}

	// Detail keeps working anonymously through the public page.
	detail, err := client.MovieDetail(context.Background(), "QD3n9")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Code != "IPX-811" || len(detail.ActorMovies) != 2 {
		t.Fatalf("web detail = %#v", detail)
	}
	if len(transport.calls) != 1 {
		t.Fatalf("detail API calls = %d, want 1", len(transport.calls))
	}
}
