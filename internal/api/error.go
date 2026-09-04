package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
)

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
		status := http.StatusInternalServerError
		var invalidRequest *requestError
		switch {
		case errors.As(err, &invalidRequest):
			status = http.StatusBadRequest
		case ent.IsNotFound(err):
			status = http.StatusNotFound
		case errors.Is(err, pan.ErrUnauthorized):
			status = http.StatusUnauthorized
		default:
			logger.ErrorContext(c.Request.Context(), "request failed",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"error", err,
			)
		}

		c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
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
