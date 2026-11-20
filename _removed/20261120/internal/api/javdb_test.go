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

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/javdb"
)

type stubJavDBManager struct {
	cfg    javdb.Config
	tested javdb.Config
	info   javdb.LoginInfo
	err    error
}

func (s *stubJavDBManager) JavDBConfig(context.Context) (javdb.Config, error) {
	return s.cfg, s.err
}

func (s *stubJavDBManager) UpdateJavDBConfig(_ context.Context, cfg javdb.Config) error {
	if s.err != nil {
		return s.err
	}
	s.cfg = cfg
	return nil
}

func (s *stubJavDBManager) TestJavDBLogin(_ context.Context, cfg javdb.Config) (javdb.LoginInfo, error) {
	s.tested = cfg
	return s.info, s.err
}

func TestJavDBLoginRequiredResponse(t *testing.T) {
	router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	router.GET("/probe", func(c *gin.Context) { c.Error(javdb.ErrLoginRequired) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "JAVDB_LOGIN_REQUIRED" || body.Error == "" {
		t.Fatalf("body = %#v", body)
	}
}

func TestJavDBEndpoints(t *testing.T) {
	stub := &stubJavDBManager{
		cfg:  javdb.Config{Username: "saved", Password: "secret"},
		info: javdb.LoginInfo{UserID: "42", Username: "tester"},
	}
	router := NewRouter(Dependencies{Access: NewAccessGateService("", ""), JavDB: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/javdb", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var got javdb.Config
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Username != "saved" {
		t.Fatalf("GET body = %s, err = %v", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	body, _ := json.Marshal(javdb.Config{Username: "next", Password: "new-pass"})
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings/javdb", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || stub.cfg.Username != "next" || stub.cfg.Password != "new-pass" {
		t.Fatalf("PUT status = %d, cfg = %#v", rec.Code, stub.cfg)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/javdb/test", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || stub.tested.Username != "next" {
		t.Fatalf("POST test status = %d, tested = %#v", rec.Code, stub.tested)
	}
	var info javdb.LoginInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil || info.UserID != "42" {
		t.Fatalf("POST test body = %s, err = %v", rec.Body.String(), err)
	}
}
