package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

func readLogRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var records []map[string]any
	for {
		var record map[string]any
		if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
			return records
		} else if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
}

func assertPrivateLogValuesAbsent(t *testing.T, logs string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(logs, secret) || strings.Contains(logs, url.QueryEscape(secret)) {
			t.Fatalf("log contains a private value: %s", logs)
		}
	}
}

func TestAccessLogsOmitQueryAndRefererWithoutChangingPlayback(t *testing.T) {
	const token = "playback-secret+/%&"
	const referer = "https://viewer.example/watch?token=referer-secret"
	const destination = "https://cdn.example/video.mp4?signature=cdn-secret"
	for _, tc := range []struct {
		name, method, token string
		status              int
	}{
		{"GET", http.MethodGet, token, http.StatusFound},
		{"HEAD", http.MethodHead, token, http.StatusOK},
		{"rejected", http.MethodGet, "invalid-secret", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := NewRouter(Dependencies{
				Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
				Access: NewAccessGateService("password", "session-secret"),
				STRM: &strmRelayStub{streamURL: destination, headStatus: http.StatusOK,
					headHeaders: http.Header{"Content-Type": {"video/mp4"}}},
				STRMToken: token,
			})
			query := "token=" + url.QueryEscape(tc.token) + "&url=" + url.QueryEscape(destination)
			req := httptest.NewRequest(tc.method, "/api/strm/play/123?"+query, nil)
			req.Header.Set("Referer", referer)
			req.Header.Set("X-Request-Id", "playback-request")
			req.Header.Set("Authorization", "Bearer header-secret")
			req.AddCookie(&http.Cookie{Name: cookieAuthToken, Value: "cookie-secret"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.status == http.StatusFound && response.Header().Get("Location") != destination {
				t.Fatal("signed playback redirect changed")
			}
			if req.URL.RawQuery != query || req.Referer() != referer || req.Header.Get("Authorization") != "Bearer header-secret" {
				t.Fatal("logging changed the original request")
			}
			assertPrivateLogValuesAbsent(t, logs.String(), token, tc.token, "referer-secret", "cdn-secret", "header-secret", "cookie-secret")
			var accessCount int
			for _, record := range readLogRecords(t, logs.Bytes()) {
				request, ok := record["request"].(map[string]any)
				if !ok {
					continue
				}
				accessCount++
				if _, ok := request["query"]; ok {
					t.Fatal("query field retained")
				}
				if _, ok := request["referer"]; ok {
					t.Fatal("referer field retained")
				}
				responseLog := record["response"].(map[string]any)
				if request["method"] != tc.method || request["path"] != "/api/strm/play/123" ||
					record["id"] != "playback-request" || responseLog["status"] != float64(tc.status) || responseLog["latency"] == nil {
					t.Fatalf("missing request diagnostics: %v", record)
				}
			}
			if accessCount != 1 {
				t.Fatalf("access log count = %d", accessCount)
			}
		})
	}
}

type loggingCatalogueStub struct {
	CatalogueManager
	address string
	err     error
}

func (s *loggingCatalogueStub) Media(_ context.Context, address string) (domain.Media, error) {
	s.address = address
	return domain.Media{ContentType: "image/jpeg", Body: []byte("image")}, s.err
}

func TestImageLoggingPreservesSourceAndRedactsErrorURLs(t *testing.T) {
	const nested = "https://signed.example/image.jpg?signature=nested-secret"
	source := "https://private-user:private-password@images.example/cover.jpg?token=image-secret&url=" + url.QueryEscape(nested) + "#fragment-secret"
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("failure=%t", fail), func(t *testing.T) {
			var logs bytes.Buffer
			catalogue := &loggingCatalogueStub{}
			wantStatus, wantBody := http.StatusOK, "image"
			if fail {
				catalogue.err = domain.E(domain.KindUpstream, "图片下载失败", fmt.Errorf("Get %q: TLS handshake timeout", source))
				wantStatus, wantBody = http.StatusBadGateway, `{"error":"图片下载失败"}`
			}
			router := NewRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
				Access: NewAccessGateService("", ""), Catalogue: catalogue})
			query := "url=" + url.QueryEscape(source)
			req := httptest.NewRequest(http.MethodGet, "/api/image?"+query, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if catalogue.address != source || req.URL.RawQuery != query || response.Code != wantStatus || response.Body.String() != wantBody {
				t.Fatalf("image behavior changed: source=%q, status=%d, body=%s", catalogue.address, response.Code, response.Body)
			}
			assertPrivateLogValuesAbsent(t, logs.String(), "private-user", "private-password", "image-secret", "nested-secret", "fragment-secret")
			records := readLogRecords(t, logs.Bytes())
			if len(records) != 1 {
				t.Fatalf("request log count = %d, want 1", len(records))
			}
			if fail {
				value, _ := records[0]["error"].(string)
				if !strings.Contains(value, "https://images.example/cover.jpg") || strings.Count(logs.String(), "TLS handshake timeout") != 1 ||
					records[0]["cause"] != nil || records[0]["msg"] != "request failed" {
					t.Fatalf("error diagnostics lost or duplicated: %v", records[0])
				}
			}
		})
	}
}

func TestRedactLogURLs(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"dial tcp 1.2.3.4:443: connection refused", "dial tcp 1.2.3.4:443: connection refused"},
		{`Get "https://host.example/path": timeout`, `Get "https://host.example/path": timeout`},
		{`Get "https://host.example/path?token=secret&other=value": timeout`, `Get "https://host.example/path?[REDACTED]": timeout`},
		{"https://user:password@host.example/path#secret", "https://host.example/path"},
		{"socks5://user:password@proxy.example:1080", "socks5://proxy.example:1080"},
		{"http://host.example/path?token=%zz", "http://host.example/path?[REDACTED]"},
		{"https://host.example/%zz?token=secret", "[REDACTED URL]"},
		{"source=https%3A%2F%2Fhost.example%2Fpath%3Ftoken%3Dsecret: failed", "source=[REDACTED URL] failed"},
		{"source=https%253a%252f%252fhost.example%252fpath%253ftoken%253dsecret", "source=[REDACTED URL]"},
		{"https://one.example/?a=secret https://two.example/?b=another", "https://one.example/?[REDACTED] https://two.example/?[REDACTED]"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := redactLogURLs(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequestLoggingCoversWrittenErrorsAndRecoveryWithoutRawDumps(t *testing.T) {
	var rawLogs bytes.Buffer
	previous := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &rawLogs
	t.Cleanup(func() { gin.DefaultErrorWriter = previous })
	for _, mode := range []string{"written error", "written success error", "panic", "panic after response", "broken pipe", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			var logs bytes.Buffer
			router := NewRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Access: NewAccessGateService("", "")})
			router.GET("/probe", func(c *gin.Context) {
				err := errors.New("Get https://host.example/path?token=error-secret: timeout")
				switch mode {
				case "written error":
					c.String(http.StatusBadGateway, "already written")
					c.Error(err)
				case "written success error":
					c.String(http.StatusOK, "already written")
					c.Error(err)
				case "panic":
					panic(err)
				case "panic after response":
					c.String(http.StatusOK, "already written")
					panic(err)
				case "broken pipe":
					panic(fmt.Errorf("%v: %w", err, syscall.EPIPE))
				case "canceled":
					c.Error(context.Canceled)
				}
			})
			req := httptest.NewRequest(http.MethodGet, "/probe?token=request-secret", nil)
			req.AddCookie(&http.Cookie{Name: cookieAuthToken, Value: "cookie-secret"})
			if mode == "canceled" {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			assertPrivateLogValuesAbsent(t, logs.String()+rawLogs.String(), "error-secret", "request-secret", "cookie-secret")
			if rawLogs.Len() != 0 {
				t.Fatalf("Gin emitted an unfiltered recovery log: %s", rawLogs.String())
			}
			records := readLogRecords(t, logs.Bytes())
			wantRecords := 1
			if mode == "canceled" {
				wantRecords = 0
			} else if strings.HasPrefix(mode, "panic") {
				wantRecords = 2 // Dedicated panic stack and the access summary share an ID.
			}
			if len(records) != wantRecords {
				t.Fatalf("log count = %d, want %d: %s", len(records), wantRecords, logs.String())
			}
			if mode != "canceled" && strings.Count(logs.String(), "timeout") != 1 {
				t.Fatalf("error diagnostics lost or duplicated: %s", logs.String())
			}
			switch mode {
			case "written error", "written success error":
				wantStatus := http.StatusBadGateway
				if mode == "written success error" {
					wantStatus = http.StatusOK
				}
				if response.Code != wantStatus || response.Body.String() != "already written" || records[0]["kind"] != "unexpected" {
					t.Fatal("written response or late error diagnostics lost")
				}
			case "panic", "panic after response":
				wantStatus := http.StatusInternalServerError
				if mode == "panic after response" {
					wantStatus = http.StatusOK
				}
				if response.Code != wantStatus || !strings.Contains(logs.String(), "stack") || !strings.Contains(logs.String(), "logging_test.go") ||
					records[0]["level"] != "ERROR" || records[0]["id"] != records[1]["id"] || records[0]["id"] == "" {
					t.Fatal("panic recovery lost response or stack trace")
				}
			case "broken pipe":
				if response.Body.Len() != 0 {
					t.Fatal("recovery wrote to a broken connection")
				}
			case "canceled":
				if response.Code != statusClientClosedRequest || logs.Len() != 0 {
					t.Fatal("cancellation no longer suppressed in access logs")
				}
			}
		})
	}
}

func TestRequestLoggingKeepsMultipleErrorsAndStatusOnlyFailures(t *testing.T) {
	for _, withErrors := range []bool{false, true} {
		t.Run(fmt.Sprintf("errors=%t", withErrors), func(t *testing.T) {
			var logs bytes.Buffer
			router := NewRouter(Dependencies{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Access: NewAccessGateService("", "")})
			router.GET("/probe", func(c *gin.Context) {
				if withErrors {
					c.Error(fmt.Errorf("first attempt: %w", errors.New("Get https://host.example/first?token=first-secret: timeout")))
					c.Error(domain.E(domain.KindUpstream, "retry failed", errors.New("Get https://host.example/second?token=second-secret: connection reset")))
				} else {
					c.AbortWithStatus(http.StatusForbidden)
				}
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
			records := readLogRecords(t, logs.Bytes())
			if len(records) != 1 || records[0]["msg"] != "request failed" || records[0]["cause"] != nil {
				t.Fatalf("unexpected request records: %v", records)
			}
			if withErrors {
				if response.Code != http.StatusBadGateway || records[0]["kind"] != "upstream" || records[0]["level"] != "ERROR" ||
					strings.Count(logs.String(), "first attempt") != 1 || strings.Count(logs.String(), "retry failed") != 1 ||
					strings.Count(logs.String(), "connection reset") != 1 {
					t.Fatalf("error chain lost or duplicated: %v", records[0])
				}
			} else if response.Code != http.StatusForbidden || records[0]["level"] != "WARN" || records[0]["error"] != nil {
				t.Fatalf("status-only failure changed: %v", records[0])
			}
			assertPrivateLogValuesAbsent(t, logs.String(), "first-secret", "second-secret")
		})
	}
}

func TestCanceledRequestLoggingRespectsLevel(t *testing.T) {
	for _, level := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
		t.Run(level.String(), func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: level}))
			router := NewRouter(Dependencies{Logger: logger, Access: NewAccessGateService("", "")})
			router.GET("/probe", func(c *gin.Context) { c.Error(context.Canceled) })
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil).WithContext(ctx))
			if response.Code != statusClientClosedRequest || response.Body.Len() != 0 {
				t.Fatal("cancellation response changed")
			}
			records := readLogRecords(t, logs.Bytes())
			if level == slog.LevelInfo {
				if len(records) != 0 {
					t.Fatalf("cancellation produced access logs: %v", records)
				}
			} else if len(records) != 1 || records[0]["level"] != "DEBUG" || records[0]["msg"] != "request canceled" || records[0]["request"] != nil {
				t.Fatalf("unexpected cancellation diagnostics: %v", records)
			}
		})
	}
}

func BenchmarkRequestLogFields(b *testing.B) {
	for _, size := range []int{128, 64 << 10} {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "Incoming request", 0)
		record.AddAttrs(slog.Group("request", "method", "GET", "path", "/api/image",
			"query", "url="+strings.Repeat("x", size), "referer", "https://viewer.example/?token=secret"),
			slog.Group("response", "status", 200, "latency", time.Millisecond), slog.String("id", "benchmark"))
		for _, filtered := range []bool{false, true} {
			b.Run(fmt.Sprintf("query_bytes=%d/filtered=%t", size, filtered), func(b *testing.B) {
				var handler slog.Handler = slog.NewJSONHandler(io.Discard, nil)
				if filtered {
					handler = &requestLogHandler{Handler: handler}
				}
				b.ReportAllocs()
				for b.Loop() {
					if err := handler.Handle(b.Context(), record); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
