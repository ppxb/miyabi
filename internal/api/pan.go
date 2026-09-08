package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/pan"
	"github.com/ppxb/miyabi/internal/service"
)

type PanManager interface {
	Account(context.Context) (service.PanAccountStatus, error)
	BeginLogin(context.Context) (service.PanLoginSession, error)
	LoginStatus(context.Context, string) (service.PanLoginStatus, error)
	Disconnect(context.Context) (service.PanAccountStatus, error)
	Files(context.Context, string, int) (pan.FilePage, error)
	SelectDirectory(context.Context, string) (service.PanLibraryDirectory, error)
	ClearDirectory(context.Context) error
}

type panLoginURI struct {
	ID string `uri:"id" binding:"required,uuid4"`
}

type panFilesQuery struct {
	DirectoryID string `form:"directory_id,default=0" binding:"number"`
	Page        int    `form:"page,default=1" binding:"min=1"`
}

type panDirectoryInput struct {
	ID string `json:"id" binding:"required,number"`
}

func panAccountHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		account, err := pan.Account(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, account)
	}
}

func panBeginLoginHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := pan.BeginLogin(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, session)
	}
}

func panLoginStatusHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri panLoginURI
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		status, err := pan.LoginStatus(c.Request.Context(), uri.ID)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func panDisconnectHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := pan.Disconnect(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func panFilesHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query panFilesQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		files, err := pan.Files(c.Request.Context(), query.DirectoryID, query.Page)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, files)
	}
}

func panSelectDirectoryHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input panDirectoryInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		directory, err := pan.SelectDirectory(c.Request.Context(), input.ID)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, directory)
	}
}

func panClearDirectoryHandler(pan PanManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := pan.ClearDirectory(c.Request.Context()); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, nil)
	}
}
