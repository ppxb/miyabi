package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/service"
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

func BadRequest(err error) error {
	return &requestError{err: err}
}

func errorMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		if errors.Is(err, context.Canceled) && errors.Is(c.Request.Context().Err(), context.Canceled) {
			c.AbortWithStatus(statusClientClosedRequest)
			logger.DebugContext(c.Request.Context(), "request canceled",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"id", sloggin.GetRequestID(c),
			)
			return
		}
		status := http.StatusInternalServerError
		var invalidRequest *requestError
		switch {
		case errors.As(err, &invalidRequest):
			status = http.StatusBadRequest
		case errors.Is(err, service.ErrMediaDirectoryRequired), errors.Is(err, service.ErrMagnetNotFound), errors.Is(err, service.ErrInvalidWatchProgress):
			status = http.StatusBadRequest
		case errors.Is(err, service.ErrWatchHistorySourceChanged):
			status = http.StatusConflict
		case ent.IsNotFound(err), errors.Is(err, fs.ErrNotExist):
			status = http.StatusNotFound
		case errors.Is(err, pan.ErrUnauthorized), errors.Is(err, service.ErrAccessPassword):
			status = http.StatusUnauthorized
		}

		message := err.Error()
		var public interface{ PublicMessage() string }
		if errors.As(err, &public) {
			message = public.PublicMessage()
		}
		c.AbortWithStatusJSON(status, gin.H{"error": message})
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
