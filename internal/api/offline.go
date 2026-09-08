package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type OfflineManager interface {
	Add(context.Context, string, string) (service.OfflineSubmission, error)
	Tasks(context.Context, string, string) ([]service.OfflineSubmission, error)
}

type offlineInput struct {
	Hash string `json:"hash" binding:"required,len=40,hexadecimal"`
}

type offlineTasksQuery struct {
	AccountID string `form:"account_id" binding:"required,number"`
}

func offlineTasksHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri movieURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var query offlineTasksQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		tasks, err := offline.Tasks(c.Request.Context(), uri.ID, query.AccountID)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, tasks)
	}
}

func offlineAddHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri movieURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var input offlineInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		submission, err := offline.Add(c.Request.Context(), uri.ID, input.Hash)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusAccepted, submission)
	}
}
