package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/offline"
)

type OfflineManager interface {
	AddForAccount(context.Context, string, string, string) (domain.OfflineSubmission, error)
	ActivityForAccount(context.Context, string) (offline.Activity, error)
	Cancel(context.Context, int) (domain.OfflineSubmission, error)
	TryNext(context.Context, int) (domain.OfflineSubmission, error)
}

func offlineControlHandler(manager OfflineManager, next bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID int `uri:"id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		var result domain.OfflineSubmission
		var err error
		if next {
			result, err = manager.TryNext(c.Request.Context(), uri.ID)
		} else {
			result, err = manager.Cancel(c.Request.Context(), uri.ID)
		}
		accepted(c, result, err)
	}
}

func offlineActivityHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		activity, err := offline.ActivityForAccount(c.Request.Context(), c.Query("account_id"))
		respond(c, activity, err)
	}
}

type offlineInput struct {
	AccountID string `json:"account_id" binding:"required"`
	Hash      string `json:"hash" binding:"required,len=40,hexadecimal"`
}

func offlineAddHandler(offline OfflineManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[movieURI](c)
		if !ok {
			return
		}
		input, ok := bindJSON[offlineInput](c)
		if !ok {
			return
		}
		submission, err := offline.AddForAccount(c.Request.Context(), input.AccountID, uri.ID, input.Hash)
		accepted(c, submission, err)
	}
}
