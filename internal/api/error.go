package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
	sloggin "github.com/samber/slog-gin"
)

// 499 distinguishes requests abandoned by the caller from server failures.
const statusClientClosedRequest = 499

type requestError struct {
	err error
}

func (err *requestError) Error() string {
	return err.err.Error()
}

func (err *requestError) Unwrap() error {
	return err.err
}

func (err *requestError) DomainKind() domain.Kind {
	return domain.KindInvalid
}

func (err *requestError) PublicMessage() string {
	var public interface{ PublicMessage() string }
	if errors.As(err.err, &public) {
		if msg := public.PublicMessage(); msg != "" {
			return msg
		}
	}
	if err.err != nil && err.err.Error() != "" {
		return err.err.Error()
	}
	return "请求参数无效"
}

func BadRequest(err error) error {
	return &requestError{err: err}
}

func httpStatusForKind(kind domain.Kind) int {
	switch kind {
	case domain.KindInvalid:
		return http.StatusBadRequest
	case domain.KindUnauthorized:
		return http.StatusUnauthorized
	case domain.KindNotFound:
		return http.StatusNotFound
	case domain.KindConflict, domain.KindBusy:
		return http.StatusConflict
	case domain.KindRateLimited:
		return http.StatusTooManyRequests
	case domain.KindUpstream:
		return http.StatusBadGateway
	case domain.KindCanceled:
		return statusClientClosedRequest
	default:
		return http.StatusInternalServerError
	}
}

func mapErrorStatus(err error) (int, domain.Kind) {
	kind := domain.KindOf(err)
	if kind != domain.KindUnexpected {
		return httpStatusForKind(kind), kind
	}
	switch {
	case ent.IsNotFound(err), errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound, domain.KindNotFound
	default:
		return http.StatusInternalServerError, domain.KindUnexpected
	}
}

func errorMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		clientCanceled := errors.Is(c.Request.Context().Err(), context.Canceled) &&
			(errors.Is(err, context.Canceled) || domain.IsKind(err, domain.KindCanceled))
		if clientCanceled {
			c.AbortWithStatus(statusClientClosedRequest)
			logger.DebugContext(c.Request.Context(), "request canceled",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"id", sloggin.GetRequestID(c),
			)
			return
		}

		status, kind := mapErrorStatus(err)
		message := domain.PublicMessage(err)
		if status == http.StatusNotFound && message == "内部服务错误" {
			message = "资源不存在"
		}

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"kind", kind.String(),
			"id", sloggin.GetRequestID(c),
			"error", err.Error(),
		}
		var de *domain.Error
		if errors.As(err, &de) && de.Cause != nil {
			attrs = append(attrs, "cause", de.Cause.Error())
		}

		if status >= http.StatusInternalServerError {
			logger.ErrorContext(c.Request.Context(), "request failed", attrs...)
		} else {
			logger.WarnContext(c.Request.Context(), "request failed", attrs...)
		}

		body := gin.H{"error": message}
		switch {
		case errors.Is(err, pan.ErrUnauthorized):
			body["code"] = "PAN_UNAUTHORIZED"
		case errors.Is(err, drive.ErrMediaDirectoryRequired):
			body["code"] = "PAN_DIRECTORY_REQUIRED"
		case errors.Is(err, drive.ErrSourceChanged):
			body["code"] = "PAN_SOURCE_CHANGED"
		}
		c.AbortWithStatusJSON(status, body)
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.ErrorContext(c.Request.Context(), "request panicked",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"error", fmt.Sprint(recovered),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	})
}
