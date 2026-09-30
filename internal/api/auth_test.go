package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/maintenance"
)

type dummyDataStub struct{}

func (dummyDataStub) Info(context.Context) (maintenance.Info, error) {
	return maintenance.Info{DataDirectory: "/test/dir"}, nil
}

func (dummyDataStub) ClearCache(context.Context) (maintenance.Info, error) {
	return maintenance.Info{}, nil
}

func TestAuth_DisabledGateAllowsAll(t *testing.T) {
	gate := NewAccessGateService("", "")
	router := NewRouter(Dependencies{
		Access:      gate,
		Maintenance: dummyDataStub{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// /api/settings/system should be accessible without any token
	req := httptest.NewRequest(http.MethodGet, "/api/settings/system", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Security headers must still be present
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing X-Content-Type-Options header")
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("missing X-Frame-Options header")
	}

	// /api/auth/config should report enabled: false, authenticated: true
	reqConfig := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	recConfig := httptest.NewRecorder()
	router.ServeHTTP(recConfig, reqConfig)

	var configResp struct {
		Enabled       bool `json:"enabled"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(recConfig.Body.Bytes(), &configResp); err != nil {
		t.Fatalf("unmarshal config resp: %v", err)
	}
	if configResp.Enabled {
		t.Fatalf("expected enabled=false")
	}
	if !configResp.Authenticated {
		t.Fatalf("expected authenticated=true when gate is disabled")
	}

	// Disabled password protection accepts login without issuing a session cookie.
	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"ignored"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, login)
	var result accessLoginResponse
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if loginResponse.Code != http.StatusOK || !result.Success || result.ExpiresAt != 0 || len(loginResponse.Result().Cookies()) != 0 {
		t.Fatalf("disabled gate login: status=%d body=%s cookies=%v", loginResponse.Code, loginResponse.Body, loginResponse.Result().Cookies())
	}
}

func TestAuth_EnabledGateEnforcesJWT(t *testing.T) {
	password := "my-secret-password-123"
	gate := NewAccessGateService(password, "")
	router := NewRouter(Dependencies{
		Access:      gate,
		Maintenance: dummyDataStub{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// 1. Unauthenticated request to protected route must be rejected with 401
	{
		req := httptest.NewRequest(http.MethodGet, "/api/settings/system", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 Unauthorized, got %d", rec.Code)
		}
	}

	// 2. /api/auth/config indicates enabled=true, authenticated=false
	{
		req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var configResp struct {
			Enabled       bool `json:"enabled"`
			Authenticated bool `json:"authenticated"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &configResp); err != nil {
			t.Fatalf("unmarshal config resp: %v", err)
		}
		if !configResp.Enabled || configResp.Authenticated {
			t.Fatalf("expected enabled=true, authenticated=false, got %+v", configResp)
		}
	}

	// 3. Login with incorrect password
	{
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong password, got %d", rec.Code)
		}
	}

	// 4. Login returns metadata; the JWT is only in the HttpOnly cookie.
	var token string
	var cookieHeader string
	{
		body, _ := json.Marshal(map[string]string{"password": password})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for correct password, got %d", rec.Code)
		}

		var loginResp struct {
			Success   bool   `json:"success"`
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
			t.Fatalf("unmarshal login resp: %v", err)
		}
		if !loginResp.Success || loginResp.Token != "" || loginResp.ExpiresAt == 0 {
			t.Fatalf("expected success without a JSON token, got %+v", loginResp)
		}
		token = rec.Result().Cookies()[0].Value
		if err := gate.VerifyToken(token); err != nil {
			t.Fatalf("cookie does not contain a valid JWT: %v", err)
		}

		cookieHeader = rec.Header().Get("Set-Cookie")
		if !strings.Contains(cookieHeader, "miyabi_token=") {
			t.Fatalf("expected Set-Cookie with miyabi_token, got %s", cookieHeader)
		}
		if !strings.Contains(cookieHeader, "HttpOnly") {
			t.Fatalf("expected HttpOnly cookie, got %s", cookieHeader)
		}
	}

	// 5. Access protected route with Authorization: Bearer <token>
	{
		req := httptest.NewRequest(http.MethodGet, "/api/settings/system", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 with Bearer token, got %d", rec.Code)
		}
	}

	// 6. Access protected route with Cookie
	{
		req := httptest.NewRequest(http.MethodGet, "/api/settings/system", nil)
		req.AddCookie(&http.Cookie{Name: "miyabi_token", Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 with Cookie, got %d", rec.Code)
		}
	}

	// 7. Access protected route with ?token= query parameter
	{
		req := httptest.NewRequest(http.MethodGet, "/api/settings/system?token="+token, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 with query token, got %d", rec.Code)
		}
	}

}

func TestAuth_RateLimiterTriggers(t *testing.T) {
	password := "correct-password"
	gate := NewAccessGateService(password, "")
	router := NewRouter(Dependencies{
		Access: gate,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Fail 5 times from same IP (httptest uses 192.0.2.1 by default or RemoteAddr)
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// 6th attempt is rate-limited with HTTP 429.
	{
		body, _ := json.Marshal(map[string]string{"password": password})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("expected blocked with StatusTooManyRequests (KindRateLimited), got %d: %s", rec.Code, rec.Body.String())
		}
	}
}

func TestAuth_UntrustedProxiesIgnoreSpoofedIP(t *testing.T) {
	password := "correct-password"
	gate := NewAccessGateService(password, "")
	// By default, TrustedProxies is empty so router.SetTrustedProxies(nil) is in effect
	router := NewRouter(Dependencies{
		Access: gate,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	// Fail 5 times with different spoofed X-Forwarded-For headers, but from the same TCP RemoteAddr
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		req.RemoteAddr = "203.0.113.50:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	// 6th attempt with another spoofed X-Forwarded-For must STILL be blocked because RemoteAddr was banned!
	{
		body, _ := json.Marshal(map[string]string{"password": password})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "198.51.100.99")
		req.RemoteAddr = "203.0.113.50:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("expected blocked with StatusTooManyRequests even with spoofed X-Forwarded-For, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}
