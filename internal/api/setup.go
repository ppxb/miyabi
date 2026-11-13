package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ppxb/miyabi/internal/setup"
)

type SetupManager interface {
	Status() setup.State
	NeedsSetup() bool
	Complete(context.Context, []string) error
	LocalDirectory(context.Context) (string, error)
	SetLocalDirectory(context.Context, string) error
}

type AccountManager interface {
	Configured() bool
	Username() string
	Set(context.Context, string, string) error
}

func setupStatusHandler(manager SetupManager, account AccountManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		state := manager.Status()
		sources := state.Sources
		if sources == nil {
			sources = []string{}
		}
		localDirectory, err := manager.LocalDirectory(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		respond(c, gin.H{
			"needs_setup":        manager.NeedsSetup(),
			"completed":          state.Completed,
			"sources":            sources,
			"local_directory":    localDirectory,
			"account_configured": account.Configured(),
		}, nil)
	}
}

type setupAccountInput struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

func setupAccountHandler(gate AccessGate, account AccountManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[setupAccountInput](c)
		if !ok {
			return
		}
		if err := account.Set(c.Request.Context(), input.Username, input.Password); err != nil {
			c.Error(err)
			return
		}
		token, expiresAt, err := gate.GenerateToken()
		if err != nil {
			c.Error(err)
			return
		}
		setAuthCookie(c, token, int(defaultTokenTTL.Seconds()))
		respond(c, accessLoginResponse{Success: true, ExpiresAt: expiresAt}, nil)
	}
}

type setupCompleteInput struct {
	Sources []string `json:"sources" binding:"required,min=1,dive"`
}

func setupCompleteHandler(manager SetupManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[setupCompleteInput](c)
		if !ok {
			return
		}
		if err := manager.Complete(c.Request.Context(), input.Sources); err != nil {
			c.Error(err)
			return
		}
		respond(c, gin.H{"completed": true}, nil)
	}
}

func setupLocalDirectoryHandler(manager SetupManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			dir, err := manager.LocalDirectory(c.Request.Context())
			if err != nil {
				c.Error(err)
				return
			}
			respond(c, gin.H{"path": dir}, nil)
			return
		}
		input, ok := bindJSON[struct {
			Path string `json:"path" binding:"required"`
		}](c)
		if !ok {
			return
		}
		if err := manager.SetLocalDirectory(c.Request.Context(), input.Path); err != nil {
			c.Error(err)
			return
		}
		dir, err := manager.LocalDirectory(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		respond(c, gin.H{"path": dir}, nil)
	}
}

// setupAccessMiddleware keeps the wizard public until an account exists, then
// requires a valid session for the remaining setup mutations.
func setupAccessMiddleware(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !gate.Enabled() {
			c.Next()
			return
		}
		token := extractToken(c)
		if token == "" || gate.VerifyToken(token) != nil {
			abortAccessUnauthorized(c, "认证令牌无效或已过期，请重新登录")
			return
		}
		c.Next()
	}
}
