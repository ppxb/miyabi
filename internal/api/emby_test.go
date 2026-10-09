package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/emby"
)

type stubEmbyManager struct {
	retryCalls  int
	configCalls int
	testCalls   int
	cfg         emby.Config
	tested      emby.Config
	info        emby.ServerInfo
	configErr   error
	err         error
}

func (s *stubEmbyManager) Config(context.Context) (emby.Config, error) {
	s.configCalls++
	if s.configErr != nil {
		return emby.Config{}, s.configErr
	}
	return s.cfg, s.err
}

func (s *stubEmbyManager) UpdateConfig(_ context.Context, cfg emby.Config) error {
	if s.err != nil {
		return s.err
	}
	s.cfg = cfg
	return nil
}

func (s *stubEmbyManager) Test(_ context.Context, cfg emby.Config) (emby.ServerInfo, error) {
	s.testCalls++
	s.tested = cfg
	return s.info, s.err
}

func TestEmbyEndpoints(t *testing.T) {
	stub := &stubEmbyManager{
		cfg: emby.Config{
			Enabled:   true,
			ServerURL: "http://192.168.1.100:8096",
			APIKey:    "test-key",
			MediaPath: "/media",
		},
		info: emby.ServerInfo{
			ServerName: "MyEmby",
			Version:    "4.8.8.0",
			ID:         "test-id",
		},
	}
	router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Emby: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	// GET /api/settings/emby
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/settings/emby", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var gotCfg emby.Config
	_ = json.Unmarshal(rec.Body.Bytes(), &gotCfg)
	if gotCfg.ServerURL != "http://192.168.1.100:8096" {
		t.Errorf("unexpected cfg: %+v", gotCfg)
	}

	// PUT /api/settings/emby
	updateBody, _ := json.Marshal(emby.Config{
		Enabled:   false,
		ServerURL: "http://192.168.1.200:8096",
	})
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/api/settings/emby", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d", rec.Code)
	}
	if stub.cfg.ServerURL != "http://192.168.1.200:8096" {
		t.Errorf("expected updated URL, got %s", stub.cfg.ServerURL)
	}

	// POST /api/settings/emby/test
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/settings/emby/test", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on test, got %d", rec.Code)
	}
	var gotInfo emby.ServerInfo
	_ = json.Unmarshal(rec.Body.Bytes(), &gotInfo)
	if gotInfo.ServerName != "MyEmby" {
		t.Errorf("unexpected server info: %+v", gotInfo)
	}

	// Error test
	stub.err = domain.E(domain.KindUpstream, "连接失败", nil)
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/api/settings/emby/test", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func (s *stubEmbyManager) RetryPending(context.Context) error { s.retryCalls++; return s.err }

func TestEmbyTestEndpointConfiguration(t *testing.T) {
	saved := emby.Config{Enabled: true, ServerURL: "http://saved.example:8096", APIKey: "saved-key", MediaPath: "/media"}
	candidate := emby.Config{ServerURL: "http://candidate.example:8096", APIKey: "candidate-key"}
	body := `{"server_url":"http://candidate.example:8096","api_key":"candidate-key"}`
	loadErr := domain.E(domain.KindUnexpected, "配置读取失败", nil)
	for _, tc := range []struct {
		name, body            string
		streamed              bool
		configErr, testErr    error
		status, reads, probes int
		want                  emby.Config
	}{
		{name: "saved", status: http.StatusOK, reads: 1, probes: 1, want: saved},
		{name: "candidate skips saved config", body: body, configErr: loadErr, status: http.StatusOK, probes: 1, want: candidate},
		{name: "streamed candidate", body: body, streamed: true, status: http.StatusOK, probes: 1, want: candidate},
		{name: "partial candidate does not merge", body: `{"server_url":"http://candidate.example:8096"}`, status: http.StatusOK, probes: 1, want: emby.Config{ServerURL: candidate.ServerURL}},
		{name: "empty object does not use saved config", body: `{}`, status: http.StatusOK, probes: 1},
		{name: "malformed JSON", body: `{`, configErr: loadErr, status: http.StatusBadRequest},
		{name: "invalid field type", body: `{"server_url":42}`, status: http.StatusBadRequest},
		{name: "saved config error", configErr: loadErr, status: http.StatusInternalServerError, reads: 1},
		{name: "probe error", body: body, testErr: domain.E(domain.KindUpstream, "连接失败", nil), status: http.StatusBadGateway, probes: 1, want: candidate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubEmbyManager{cfg: saved, info: emby.ServerInfo{ServerName: "TestEmby"}, configErr: tc.configErr, err: tc.testErr}
			router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Emby: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			request := httptest.NewRequest(http.MethodPost, "/api/settings/emby/test", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			if tc.streamed {
				request.ContentLength = -1
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d, want %d: %s", response.Code, tc.status, response.Body)
			}
			if stub.configCalls != tc.reads || stub.testCalls != tc.probes || stub.tested != tc.want {
				t.Fatalf("reads=%d probes=%d tested=%+v", stub.configCalls, stub.testCalls, stub.tested)
			}
			if stub.cfg != saved {
				t.Fatal("test request changed saved configuration")
			}
			if tc.status == http.StatusOK {
				var info emby.ServerInfo
				if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil || info != stub.info {
					t.Fatalf("unexpected test response: %s (%v)", response.Body, err)
				}
			}
		})
	}
}
