package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	cookieAuthToken = "miyabi_token"
	defaultTokenTTL = 30 * 24 * time.Hour
)

type AccessGate interface {
	Enabled() bool
	Verify(string) error
	GenerateToken() (string, int64, error)
	VerifyToken(string) error
}

type accessLoginInput struct {
	Password string `json:"password" binding:"required"`
}

type accessLoginResponse struct {
	Success   bool  `json:"success"`
	ExpiresAt int64 `json:"expires_at,omitempty"`
}

func accessConfigHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !gate.Enabled() {
			respond(c, gin.H{"enabled": false, "authenticated": true}, nil)
			return
		}

		authenticated := false
		token := extractToken(c)
		if token != "" {
			if err := gate.VerifyToken(token); err == nil {
				authenticated = true
			}
		}

		respond(c, gin.H{
			"enabled":       true,
			"authenticated": authenticated,
		}, nil)
	}
}

func accessLoginHandler(gate AccessGate, limiter *loginRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		attempt, err := limiter.begin(ip)
		if err != nil {
			c.Error(err)
			return
		}
		result := loginNotVerified
		defer func() { limiter.finish(ip, attempt, result) }()

		input, ok := bindJSON[accessLoginInput](c)
		if !ok {
			return
		}

		if err := gate.Verify(input.Password); err != nil {
			result = loginFailed
			c.Error(err)
			return
		}

		result = loginSucceeded

		var token string
		var expiresAt int64
		if gate.Enabled() {
			var genErr error
			token, expiresAt, genErr = gate.GenerateToken()
			if genErr != nil {
				c.Error(genErr)
				return
			}

			setAuthCookie(c, token, int(defaultTokenTTL.Seconds()))
		}

		respond(c, accessLoginResponse{
			Success:   true,
			ExpiresAt: expiresAt,
		}, nil)
	}
}

func authMiddleware(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !gate.Enabled() {
			c.Next()
			return
		}

		token := extractToken(c)
		if token == "" {
			abortAccessUnauthorized(c, "未提供认证令牌，请登录")
			return
		}

		if err := gate.VerifyToken(token); err != nil {
			abortAccessUnauthorized(c, "认证令牌无效或已过期，请重新登录")
			return
		}

		c.Next()
	}
}

func abortAccessUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": message,
		"code":  "UNAUTHORIZED",
	})
}

func extractToken(c *gin.Context) string {
	token, _ := c.Cookie(cookieAuthToken)
	return token
}

func sameOriginMiddleware() gin.HandlerFunc {
	protection := http.NewCrossOriginProtection()
	return func(c *gin.Context) {
		if err := protection.Check(c.Request); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "不允许跨来源修改请求", "code": "CROSS_ORIGIN_DENIED"})
			return
		}
		c.Next()
	}
}

func setAuthCookie(c *gin.Context, token string, maxAge int) {
	secure := c.Request.TLS != nil || strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieAuthToken, token, maxAge, "/", "", secure, true)
}
