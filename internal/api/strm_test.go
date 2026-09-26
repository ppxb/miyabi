package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type strmRelayStub struct {
	streamURL   string
	streamErr   error
	headHeaders http.Header
	headStatus  int
	headErr     error
}

func (s *strmRelayStub) StreamURL(ctx context.Context, fileID, userAgent string) (string, error) {
	if s.streamErr != nil {
		return "", s.streamErr
	}
	return s.streamURL, nil
}

func (s *strmRelayStub) Probe(ctx context.Context, address string, headers http.Header) (*http.Response, error) {
	if s.headErr != nil {
		return nil, s.headErr
	}
	res := &http.Response{
		StatusCode: s.headStatus,
		Header:     s.headHeaders,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	if res.StatusCode == 0 {
		res.StatusCode = http.StatusOK
	}
	return res, nil
}

func TestSTRMStreamHandlerRedirectsGET(t *testing.T) {
	stub := &strmRelayStub{
		streamURL: "https://cdn.115.com/video/original.mp4?token=sig",
	}
	router := NewRouter(Dependencies{
		STRM:   stub,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/strm/play/12345", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected status 302 Found, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "https://cdn.115.com/video/original.mp4?token=sig" {
		t.Fatalf("expected Location %s, got %s", stub.streamURL, location)
	}
}

func TestSTRMStreamHandlerForwardsHEAD(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "video/mp4")
	headers.Set("Content-Length", "104857600")
	headers.Set("Accept-Ranges", "bytes")

	stub := &strmRelayStub{
		streamURL:   "https://cdn.115.com/video/original.mp4",
		headHeaders: headers,
		headStatus:  http.StatusOK,
	}
	router := NewRouter(Dependencies{
		STRM:   stub,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/api/strm/play/12345", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("expected Content-Type video/mp4, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Content-Length") != "104857600" {
		t.Fatalf("expected Content-Length 104857600, got %s", rec.Header().Get("Content-Length"))
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected Accept-Ranges bytes, got %s", rec.Header().Get("Accept-Ranges"))
	}
}

func TestSTRMStreamHandlerTokenAuthentication(t *testing.T) {
	stub := &strmRelayStub{
		streamURL: "https://cdn.115.com/video/original.mp4",
	}
	router := NewRouter(Dependencies{
		STRM:      stub,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		STRMToken: "secret123",
	})

	// Missing token
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/strm/play/12345", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for missing token, got %d", rec.Code)
		}
	}

	// Wrong token
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/strm/play/12345?token=wrong", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong token, got %d", rec.Code)
		}
	}

	// Correct token
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/strm/play/12345?token=secret123", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302 for valid token, got %d", rec.Code)
		}
		if location := rec.Header().Get("Location"); location != stub.streamURL {
			t.Fatalf("expected Location %s, got %s", stub.streamURL, location)
		}
	}
}

func TestSTRMStreamHandlerAcceptsSignedInSessionsBehindTheAccessGate(t *testing.T) {
	stub := &strmRelayStub{streamURL: "https://cdn.115.com/video/original.mp4"}
	gate := NewAccessGateService("password", "secret")
	router := NewRouter(Dependencies{
		STRM:      stub,
		Access:    gate,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		STRMToken: "secret123",
	})
	session, _, err := gate.GenerateToken("miyabi", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, path, authorization string
		status                    int
	}{
		{name: "anonymous", path: "/api/strm/play/12345", status: http.StatusUnauthorized},
		{name: "strm token", path: "/api/strm/play/12345?token=secret123", status: http.StatusFound},
		{name: "signed in", path: "/api/strm/play/12345", authorization: "Bearer " + session, status: http.StatusFound},
		{name: "forged session", path: "/api/strm/play/12345", authorization: "Bearer forged", status: http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, scenario.path, nil)
			if scenario.authorization != "" {
				request.Header.Set("Authorization", scenario.authorization)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status = %d, want %d", response.Code, scenario.status)
			}
		})
	}
}
