package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type TaskManager interface {
	Revisions() service.TaskRevisions
	List(context.Context) ([]service.TaskInfo, error)
	Subscribe() (<-chan struct{}, func())
}

func tasksHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := tasks.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, result)
	}
}
