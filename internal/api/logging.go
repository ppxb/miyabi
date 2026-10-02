package api

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

func requestLoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	filtered := slog.New(&requestLogHandler{Handler: logger.Handler()})
	return sloggin.NewWithFilters(filtered, sloggin.IgnoreStatus(statusClientClosedRequest))
}

// Only the access logger uses this handler. Successful requests filter fields
// by name without inspecting query strings or parsing URLs.
type requestLogHandler struct {
	slog.Handler
}

func (h *requestLogHandler) Handle(ctx context.Context, record slog.Record) error {
	message := record.Message
	// slog-gin puts c.Errors in the message for failed HTTP requests.
	if record.Level >= slog.LevelWarn {
		message = redactLogURLs(message)
	}
	filtered := slog.NewRecord(record.Time, record.Level, message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "request" && attr.Value.Kind() == slog.KindGroup {
			fields := attr.Value.Group()
			kept := make([]slog.Attr, 0, len(fields))
			for _, field := range fields {
				if field.Key != "query" && field.Key != "referer" {
					kept = append(kept, field)
				}
			}
			attr.Value = slog.GroupValue(kept...)
		}
		filtered.AddAttrs(attr)
		return true
	})
	return h.Handler.Handle(ctx, filtered)
}

func (h *requestLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &requestLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *requestLogHandler) WithGroup(name string) slog.Handler {
	return &requestLogHandler{Handler: h.Handler.WithGroup(name)}
}

// Encoded URLs can appear inside validation errors without an outer URL.
var logURLPattern = regexp.MustCompile(`(?i)\b(?:https?|socks5h?)(?:://|%(?:25)*3a%(?:25)*2f%(?:25)*2f)[^\s"'<>]+`)

// Error paths retain the operation, host and path, but not URL credentials,
// query parameters (including nested URLs) or fragments. Requests are untouched.
func redactLogURLs(message string) string {
	return logURLPattern.ReplaceAllStringFunc(message, func(raw string) string {
		if !strings.Contains(raw, "://") {
			return "[REDACTED URL]"
		}
		address, err := url.Parse(raw)
		if err != nil {
			return "[REDACTED URL]"
		}
		if address.User == nil && address.RawQuery == "" && address.Fragment == "" {
			return raw
		}
		address.User = nil
		if address.RawQuery != "" {
			address.RawQuery = "[REDACTED]"
		}
		address.Fragment, address.RawFragment = "", ""
		return address.String()
	})
}
