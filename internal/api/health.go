package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Status string `json:"status"`
}

func healthHandler(checker HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := checker.Ping(c.Request.Context()); err != nil {
			c.Error(fmt.Errorf("check database health: %w", err))
			return
		}
		c.JSON(http.StatusOK, healthResponse{Status: "ok"})
	}
}
