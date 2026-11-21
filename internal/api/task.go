package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/tasks"
)

type TaskManager interface {
	Revisions() tasks.TaskRevisions
	Instance() string
	List(context.Context) ([]domain.TaskInfo, error)
	Retry(context.Context, int) (domain.TaskInfo, error)
	SetLibraryPaused(context.Context, bool) error
	Subscribe() (<-chan struct{}, func())
}

func taskLibraryControlHandler(manager TaskManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[struct {
			Paused *bool `json:"paused" binding:"required"`
		}](c)
		if !ok {
			return
		}
		if err := manager.SetLibraryPaused(c.Request.Context(), *input.Paused); err != nil {
			respond(c, nil, err)
			return
		}
		accepted(c, gin.H{"paused": *input.Paused}, nil)
	}
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
		accepted(c, result, err)
	}
}
