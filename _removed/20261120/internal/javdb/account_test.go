package javdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/netx"
)

type authRetryTransport struct {
	calls   int
	success []byte
}

func (t *authRetryTransport) getJSON(_ context.Context, _ string, _ url.Values, destination any) error {
	t.calls++
	if t.calls == 1 {
		return decodeEnvelope(webAuthRequiredEnvelope, destination)
	}
	return decodeEnvelope(t.success, destination)
}

func (*authRetryTransport) closeIdleConnections() {}

func TestTransportAddsBearerToken(t *testing.T) {
	client := &Client{token: "tok-123"}
	transport := &transport{host: "https://api.example", deviceUUID: "device", token: client.accessToken}
	request, err := transport.newRequest(context.Background(), "/api/v1/startup", nil, 1700000000)
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Header.Get("authorization"); got != "Bearer tok-123" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestSearchLogsInAndRetriesWithAccount(t *testing.T) {
	var credentials string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions" {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		credentials = r.PostForm.Get("username") + ":" + r.PostForm.Get("password")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"success":1,"action":null,"message":null,"data":{"token":"tok-abc","user":{"id":42,"name":"tester"}}}`))
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

	transport := &authRetryTransport{success: fixtureFile(t, "search.json")}
	client := clientWithTransport(transport)
	client.fingerprint = fingerprint
	client.SetCredentials("user1", "pass1")

	movies, err := client.Search(context.Background(), "ABP-123", domain.SearchOptions{Page: 1, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 2 || movies[1].ID != "movie-exact" {
		t.Fatalf("movies = %#v", movies)
	}
	if credentials != "user1:pass1" {
		t.Fatalf("login credentials = %q", credentials)
	}
	if transport.calls != 2 {
		t.Fatalf("API calls = %d, want 2", transport.calls)
	}
	if token := client.accessToken(); token != "tok-abc" {
		t.Fatalf("token = %q", token)
	}
	if username, password := client.Credentials(); username != "user1" || password != "pass1" {
		t.Fatalf("credentials = %q/%q", username, password)
	}
}

func TestLoginInfoParsesUser(t *testing.T) {
	info := loginInfo("fallback", json.RawMessage(`{"id":42,"name":"tester"}`))
	if info.UserID != "42" || info.Username != "tester" {
		t.Fatalf("info = %#v", info)
	}
	if empty := loginInfo("fallback", nil); empty.Username != "fallback" || empty.UserID != "" {
		t.Fatalf("empty info = %#v", empty)
	}
}
