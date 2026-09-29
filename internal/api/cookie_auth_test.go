package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestCookieAuthentication(t *testing.T) {
	gate := NewAccessGateService("password", "secret")
	valid, _, err := gate.GenerateToken("admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expired, _, err := gate.GenerateToken("admin", -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cookie, bearer, query string
		status                      int
	}{
		{"cookie", valid, "", "", 204},
		{"stale bearer cannot override cookie", valid, "expired", "", 204},
		{"anonymous", "", "", "", 401},
		{"bearer only", "", valid, "", 401},
		{"URL JWT only", "", "", valid, 401},
		{"forged cookie", "forged", "", "", 401},
		{"expired cookie", expired, "", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/cookie-test?token="+tc.query, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: cookieAuthToken, Value: tc.cookie})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			rec := httptest.NewRecorder()
			router := NewRouter(Dependencies{Access: gate, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			// Test the auth boundary independently of downstream resource implementations.
			router.GET("/cookie-test", authMiddleware(gate), func(c *gin.Context) { c.Status(204) })
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			req.URL.Path = "/api/auth/config"
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			var config struct {
				Authenticated bool `json:"authenticated"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil || config.Authenticated != (tc.status == 204) {
				t.Fatalf("incorrect session status: %s (%v)", rec.Body.String(), err)
			}
		})
	}
}

func TestCookieFlags(t *testing.T) {
	for _, target := range []string{"http://miyabi.test", "https://miyabi.test"} {
		t.Run(target, func(t *testing.T) {
			router := NewRouter(Dependencies{Access: NewAccessGateService("password"), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			req := httptest.NewRequest(http.MethodPost, target+"/api/auth/login", strings.NewReader(`{"password":"password"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			cookies := rec.Result().Cookies()
			if rec.Code != 200 || len(cookies) != 1 {
				t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
			}
			cookie := cookies[0]
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge <= 0 || cookie.Secure != strings.HasPrefix(target, "https:") {
				t.Fatalf("cookie: %+v", cookie)
			}
		})
	}
}

func TestCrossOriginMutations(t *testing.T) {
	for _, path := range []string{"/api/auth/logout", "/api/discover/viewed", "/api/auth/login"} {
		for _, tc := range []struct {
			name, site, origin string
			allowed            bool
		}{
			{"same origin", "same-origin", "http://miyabi.test", true},
			{"same origin fallback", "", "http://miyabi.test", true},
			{"non browser", "", "", true},
			{"cross site", "cross-site", "https://evil.test", false},
			{"sibling origin", "same-site", "http://other.miyabi.test", false},
			{"foreign origin fallback", "", "https://evil.test", false},
			{"opaque origin", "", "null", false},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				router := NewRouter(Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
				req := httptest.NewRequest(http.MethodPost, "http://miyabi.test"+path, nil)
				req.Header.Set("Sec-Fetch-Site", tc.site)
				req.Header.Set("Origin", tc.origin)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				want := http.StatusForbidden
				if tc.allowed {
					want = http.StatusBadRequest // Empty login/history bodies fail validation after the origin check.
					if path == "/api/auth/logout" {
						want = http.StatusOK
					}
				}
				if rec.Code != want {
					t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}
