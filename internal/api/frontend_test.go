package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestFrontendFilesRoutesAndCachePolicy(t *testing.T) {
	const index = "<!doctype html><html>application</html>"
	const script = "console.log('application')"
	frontend := fstest.MapFS{
		"index.html":                      {Data: []byte(index)},
		"assets/index-DAb_c-12.js":        {Data: []byte(script)},
		"assets/style-1234abcd.css":       {Data: []byte("body { color: red }")},
		"assets/font-ABC_def-.woff2":      {Data: []byte("font data")},
		"assets/nested/chunk-abcdefgh.js": {Data: []byte(script)},
		"assets/logo.svg":                 {Data: []byte("<svg/>")},
		"assets/chunk-short.js":           {Data: []byte(script)},
		"logo-1234abcd.svg":               {Data: []byte("<svg/>")},
		"robots.txt":                      {Data: []byte("User-agent: *")},
		"docs/index.html":                 {Data: []byte("directory index")},
	}
	router := NewRouter(Dependencies{
		Access: NewAccessGateService("", ""), Frontend: frontend,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	const immutable = "public, max-age=31536000, immutable"
	for _, test := range []struct {
		method, path string
		status       int
		cache, body  string
	}{
		{http.MethodGet, "/", 200, "no-cache", index},
		{http.MethodGet, "/index.html", 200, "no-cache", index},
		{http.MethodGet, "/discover", 200, "no-cache", index},
		{http.MethodGet, "/discover/search?query=test", 200, "no-cache", index},
		{http.MethodGet, "/subscriptions/", 200, "no-cache", index},
		{http.MethodHead, "/", 200, "no-cache", ""},
		{http.MethodHead, "/settings", 200, "no-cache", ""},
		{http.MethodGet, "/assets/index-DAb_c-12.js", 200, immutable, script},
		{http.MethodHead, "/assets/index-DAb_c-12.js", 200, immutable, ""},
		{http.MethodGet, "/assets/style-1234abcd.css", 200, immutable, "body { color: red }"},
		{http.MethodGet, "/assets/font-ABC_def-.woff2", 200, immutable, "font data"},
		{http.MethodGet, "/assets/nested/chunk-abcdefgh.js", 200, immutable, script},
		{http.MethodGet, "/assets/logo.svg", 200, "no-cache", "<svg/>"},
		{http.MethodGet, "/assets/chunk-short.js", 200, "no-cache", script},
		{http.MethodGet, "/logo-1234abcd.svg", 200, "no-cache", "<svg/>"},
		{http.MethodGet, "/robots.txt", 200, "no-cache", "User-agent: *"},
		{http.MethodGet, "/assets", 404, "no-cache", ""},
		{http.MethodGet, "/assets/", 404, "no-cache", ""},
		{http.MethodHead, "/assets/", 404, "no-cache", ""},
		{http.MethodGet, "/docs", 404, "no-cache", ""},
		{http.MethodGet, "/docs/", 404, "no-cache", ""},
		{http.MethodGet, "/assets/missing-1234abcd.js", 404, "no-cache", ""},
		{http.MethodGet, "/assets/missing", 404, "no-cache", ""},
		{http.MethodGet, "/favicon.ico", 404, "no-cache", ""},
		{http.MethodGet, "/missing.css", 404, "no-cache", ""},
		{http.MethodGet, "/../index.html", 404, "no-cache", ""},
		{http.MethodPost, "/discover", 405, "no-cache", ""},
		{http.MethodPost, "/assets/index-DAb_c-12.js", 405, "no-cache", ""},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			originalPath := req.URL.Path
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != test.status || response.Header().Get("Cache-Control") != test.cache || response.Body.String() != test.body {
				t.Fatalf("status=%d cache=%q body=%q", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
			}
			if test.status == 405 && response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatal("method rejection omitted the allowed methods")
			}
			if test.method == http.MethodHead && test.status == 200 && response.Header().Get("Content-Length") == "" {
				t.Fatal("HEAD omitted content length")
			}
			if req.URL.Path != originalPath {
				t.Fatal("SPA fallback changed the request path seen by access logging")
			}
		})
	}
	for _, path := range []string{"/api", "/api/missing"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 404 || !strings.Contains(response.Header().Get("Content-Type"), "application/json") || strings.Contains(response.Body.String(), index) {
			t.Fatalf("API request received SPA fallback: %s %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestFrontendWithoutIndexDoesNotListRoot(t *testing.T) {
	router := NewRouter(Dependencies{
		Access: NewAccessGateService("", ""), Frontend: fstest.MapFS{"assets/app-1234abcd.js": {Data: []byte("script")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for _, path := range []string{"/", "/index.html", "/discover"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 404 || response.Body.Len() != 0 {
			t.Fatalf("missing entry listed the embedded directory: %s %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestFrontendConditionalAndRangeRequests(t *testing.T) {
	modified := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	frontend := fstest.MapFS{
		"index.html":             {Data: []byte("<!doctype html>current entry"), ModTime: modified},
		"assets/app-1234abcd.js": {Data: []byte("0123456789"), ModTime: modified},
	}
	router := NewRouter(Dependencies{
		Access: NewAccessGateService("", ""), Frontend: frontend,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request := func(path, header, value string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(header, value)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	response := request("/assets/app-1234abcd.js", "If-Modified-Since", modified.Format(http.TimeFormat))
	if response.Code != http.StatusNotModified || response.Body.Len() != 0 || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("conditional asset: status=%d headers=%v", response.Code, response.Header())
	}
	response = request("/assets/app-1234abcd.js", "Range", "bytes=2-5")
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" || response.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range asset: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	response = request("/discover", "If-Modified-Since", modified.Add(24*time.Hour).Format(http.TimeFormat))
	if response.Code != http.StatusOK || response.Body.String() != string(frontend["index.html"].Data) || response.Header().Get("Cache-Control") != "no-cache" || response.Header().Get("Content-Length") != strconv.Itoa(len(frontend["index.html"].Data)) {
		t.Fatalf("entry revalidation retained an old page: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}
