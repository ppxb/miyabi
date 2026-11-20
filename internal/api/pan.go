package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type DriveManager interface {
	Accounts(context.Context) ([]drive.AccountStatus, error)
	AccountStatusFor(context.Context, string) (drive.AccountStatus, error)
	BeginLogin(context.Context) (drive.LoginSession, error)
	LoginStatus(context.Context, string) (drive.LoginStatus, error)
	DisconnectAccount(context.Context, string) error
	FilesFor(context.Context, string, string, int) (pan.FilePage, error)
	SelectDirectoryForAccount(context.Context, string, string) (domain.LibraryDirectory, error)
	ClearDirectoryForAccount(context.Context, string) error
}

type panLoginURI struct {
	ID string `uri:"id" binding:"required,uuid4"`
}

type panAccountURI struct {
	AccountID string `uri:"accountID" binding:"required"`
}

type panFilesQuery struct {
	DirectoryID string `form:"directory_id,default=0" binding:"number"`
	Page        int    `form:"page,default=1" binding:"min=1"`
}

type panDirectoryInput struct {
	ID string `json:"id" binding:"required,number"`
}

func panAccountsHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		accounts, err := pan.Accounts(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		result := make([]panAccountStatusResponse, len(accounts))
		for i, account := range accounts {
			result[i] = accountResponse(account)
		}
		respond(c, result, nil)
	}
}

func panAccountStatusHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panAccountURI](c)
		if !ok {
			return
		}
		status, err := pan.AccountStatusFor(c.Request.Context(), uri.AccountID)
		respond(c, accountResponse(status), err)
	}
}

func panAccountFilesHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panAccountURI](c)
		if !ok {
			return
		}
		query, ok := bindQuery[panFilesQuery](c)
		if !ok {
			return
		}
		files, err := pan.FilesFor(c.Request.Context(), uri.AccountID, query.DirectoryID, query.Page)
		respond(c, filesResponse(files), err)
	}
}

func panAccountSelectDirectoryHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panAccountURI](c)
		if !ok {
			return
		}
		input, ok := bindJSON[panDirectoryInput](c)
		if !ok {
			return
		}
		directory, err := pan.SelectDirectoryForAccount(c.Request.Context(), uri.AccountID, input.ID)
		respond(c, directory, err)
	}
}

func panAccountClearDirectoryHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panAccountURI](c)
		if !ok {
			return
		}
		respond(c, nil, pan.ClearDirectoryForAccount(c.Request.Context(), uri.AccountID))
	}
}

func panAccountDisconnectHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panAccountURI](c)
		if !ok {
			return
		}
		respond(c, nil, pan.DisconnectAccount(c.Request.Context(), uri.AccountID))
	}
}

func panBeginLoginHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := pan.BeginLogin(c.Request.Context())
		respond(c, session, err)
	}
}

func panLoginStatusHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[panLoginURI](c)
		if !ok {
			return
		}
		status, err := pan.LoginStatus(c.Request.Context(), uri.ID)
		respond(c, status, err)
	}
}
