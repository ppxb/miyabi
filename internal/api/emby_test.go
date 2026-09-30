package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/emby"
)

type stubEmbyManager struct {
	retryCalls int
	cfg        emby.Config
	info       emby.ServerInfo
	err        error
}

func (s *stubEmbyManager) Config(context.Context) (emby.Config, error) {
	return s.cfg, s.err
}

func (s *stubEmbyManager) UpdateConfig(_ context.Context, cfg emby.Config) error {
	if s.err != nil {
		return s.err
	}
	s.cfg = cfg
	return nil
}

func (s *stubEmbyManager) Test(context.Context, emby.Config) (emby.ServerInfo, error) {
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
