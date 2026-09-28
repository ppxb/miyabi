package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

type TaskManager interface {
	Revisions() tasks.TaskRevisions
	List(context.Context) ([]domain.TaskInfo, error)
	Subscribe() (<-chan struct{}, func())
}

func tasksHandler(tasks TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := tasks.List(c.Request.Context())
		respond(c, result, err)
	}
}
