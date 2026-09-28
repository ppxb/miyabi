package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

type TaskManager interface {
	Revisions() tasks.TaskRevisions
	List(context.Context) ([]domain.TaskInfo, error)
	Retry(context.Context, int) (domain.TaskInfo, error)
	Subscribe() (<-chan struct{}, func())
}

func tasksHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := tasks.List(c.Request.Context())
		respond(c, result, err)
	}
}

func taskRetryHandler(manager TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID int `uri:"id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		result, err := manager.Retry(c.Request.Context(), uri.ID)
		if err != nil {
			respond(c, nil, err)
			return
		}
		c.JSON(http.StatusAccepted, result)
	}
}
