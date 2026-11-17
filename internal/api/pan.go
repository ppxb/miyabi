package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/pan"
)

type DriveManager interface {
	Account(context.Context) (drive.AccountStatus, error)
	Accounts(context.Context) ([]drive.AccountStatus, error)
	AccountStatusFor(context.Context, string) (drive.AccountStatus, error)
	BeginLogin(context.Context) (drive.LoginSession, error)
	LoginStatus(context.Context, string) (drive.LoginStatus, error)
	Disconnect(context.Context) (drive.AccountStatus, error)
	DisconnectAccount(context.Context, string) error
	Files(context.Context, string, int) (pan.FilePage, error)
	FilesFor(context.Context, string, string, int) (pan.FilePage, error)
	SelectDirectory(context.Context, string) (domain.LibraryDirectory, error)
	SelectDirectoryForAccount(context.Context, string, string) (domain.LibraryDirectory, error)
	ClearDirectory(context.Context) error
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

func panAccountHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		account, err := pan.Account(c.Request.Context())
		respond(c, accountResponse(account), err)
	}
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

func panDisconnectHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := pan.Disconnect(c.Request.Context())
		respond(c, accountResponse(status), err)
	}
}

func panFilesHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[panFilesQuery](c)
		if !ok {
			return
		}
		files, err := pan.Files(c.Request.Context(), query.DirectoryID, query.Page)
		respond(c, filesResponse(files), err)
	}
}

func panSelectDirectoryHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[panDirectoryInput](c)
		if !ok {
			return
		}
		directory, err := pan.SelectDirectory(c.Request.Context(), input.ID)
		respond(c, directory, err)
	}
}

func panClearDirectoryHandler(pan DriveManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := pan.ClearDirectory(c.Request.Context())
		respond(c, nil, err)
	}
}
