package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/maintenance"
	"github.com/ppxb/miyabi/internal/netx"
	"github.com/ppxb/miyabi/internal/offline"
	"github.com/ppxb/miyabi/internal/pan"
)

type publicError struct{ message string }

func (err *publicError) Error() string         { return "internal detail: " + err.message }
func (err *publicError) PublicMessage() string { return err.message }

// proxyValidationError returns the real error produced by netx for a proxy
// URL without a scheme, so the test pins the wire-level contract of that path.
func proxyValidationError() error {
	_, err := netx.NewProxyManager(netx.ProxyConfig{Enabled: true, URL: "127.0.0.1"})
	return err
}

func errorRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(errorMiddleware(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/probe", handler)
	return router
}

func TestErrorMiddlewareMapsDomainErrorsToStatusAndMessage(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{name: "bad request wrapper", err: BadRequest(errors.New("page must be positive")), status: http.StatusBadRequest, message: "page must be positive"},
		{name: "media directory required", err: drive.ErrMediaDirectoryRequired, status: http.StatusBadRequest, message: drive.ErrMediaDirectoryRequired.PublicMessage()},
		{name: "magnet not found", err: fmt.Errorf("add: %w", offline.ErrMagnetNotFound), status: http.StatusBadRequest, message: offline.ErrMagnetNotFound.PublicMessage()},
		{name: "invalid proxy", err: proxyValidationError(), status: http.StatusBadRequest, message: "代理配置无效: 代理地址必须包含协议（如 http://）与主机地址"},
		{name: "cache busy", err: maintenance.ErrCacheBusy, status: http.StatusConflict, message: maintenance.ErrCacheBusy.PublicMessage()},
		{name: "file missing", err: fmt.Errorf("影片文件不存在，请重新扫描: %w", fs.ErrNotExist), status: http.StatusNotFound, message: "影片文件不存在，请重新扫描: file does not exist"},
		{name: "pan unauthorized", err: fmt.Errorf("list: %w", pan.ErrUnauthorized), status: http.StatusUnauthorized, message: pan.ErrUnauthorized.PublicMessage()},
		{name: "access password", err: ErrAccessPassword, status: http.StatusUnauthorized, message: ErrAccessPassword.PublicMessage()},
		{name: "upstream gateway error", err: domain.E(domain.KindUpstream, "上游服务异常", errors.New("javdb timeout")), status: http.StatusBadGateway, message: "上游服务异常"},

		{name: "javdb api error", err: fmt.Errorf("get JavDB movie detail: %w", &javdb.APIError{Action: "movie", Message: "not found"}), status: http.StatusBadGateway, message: "JavDB 返回了错误：not found"},
		{name: "javdb http error", err: fmt.Errorf("search JavDB: %w", &javdb.HTTPError{StatusCode: 503}), status: http.StatusBadGateway, message: "JavDB 服务异常（HTTP 503），请稍后重试"},
		{name: "public message wins", err: fmt.Errorf("wrapped: %w", &publicError{message: "115 说明文案"}), status: http.StatusInternalServerError, message: "115 说明文案"},
		{name: "unknown error leaks text", err: errors.New("UNIQUE constraint failed: movies.code"), status: http.StatusInternalServerError, message: "UNIQUE constraint failed: movies.code"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			router := errorRouter(func(c *gin.Context) { c.Error(scenario.err) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
			if response.Code != scenario.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, scenario.status, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %v\n%s", err, response.Body)
			}
			if body.Error != scenario.message {
				t.Fatalf("message = %q, want %q", body.Error, scenario.message)
			}
		})
	}
}

func TestErrorMiddlewareLogsCauseAndSeparatesPublicMessage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
		wantLevel  string
		wantCause  string
	}{
		{
			name:       "4xx error logs warn and cause",
			err:        domain.E(domain.KindInvalid, "客户端参数无效", errors.New("内部校验细节: id 不合法")),
			wantStatus: http.StatusBadRequest,
			wantMsg:    "客户端参数无效",
			wantLevel:  "WARN",
			wantCause:  "内部校验细节: id 不合法",
		},
		{
			name:       "5xx error logs error and cause",
			err:        domain.E(domain.KindUpstream, "上游网关异常", errors.New("连接超时: dial tcp 1.2.3.4:443")),
			wantStatus: http.StatusBadGateway,
			wantMsg:    "上游网关异常",
			wantLevel:  "ERROR",
			wantCause:  "连接超时: dial tcp 1.2.3.4:443",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			gin.SetMode(gin.ReleaseMode)
			router := gin.New()
			router.Use(errorMiddleware(logger))
			router.GET("/probe", func(c *gin.Context) { c.Error(tc.err) })

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("response body not json: %v", err)
			}
			if body.Error != tc.wantMsg {
				t.Fatalf("body error = %q, want %q", body.Error, tc.wantMsg)
			}

			logOutput := logBuf.String()
			if !strings.Contains(logOutput, tc.wantCause) {
				t.Fatalf("log output missing cause %q: %s", tc.wantCause, logOutput)
			}
			if !strings.Contains(logOutput, tc.wantLevel) {
				t.Fatalf("log output missing level %s: %s", tc.wantLevel, logOutput)
			}
			if strings.Contains(response.Body.String(), tc.wantCause) {
				t.Fatalf("response body leaked cause %q: %s", tc.wantCause, response.Body.String())
			}
		})
	}
}

func TestErrorMiddlewareSkipsWrittenResponsesAndCanceledRequests(t *testing.T) {
	t.Run("written response is kept", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) {
			c.JSON(http.StatusAccepted, gin.H{"ok": true})
			c.Error(errors.New("late failure"))
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if response.Code != http.StatusAccepted || response.Body.String() != `{"ok":true}` {
			t.Fatalf("response = %d %s", response.Code, response.Body)
		}
	})
	t.Run("client cancellation becomes 499", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) { c.Error(fmt.Errorf("stream: %w", context.Canceled)) })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil).WithContext(ctx))
		if response.Code != statusClientClosedRequest || response.Body.Len() != 0 {
			t.Fatalf("response = %d %s", response.Code, response.Body)
		}
	})
	t.Run("canceled error on a live request is a server failure", func(t *testing.T) {
		router := errorRouter(func(c *gin.Context) { c.Error(context.Canceled) })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d", response.Code)
		}
	})
}
