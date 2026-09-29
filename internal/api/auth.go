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
	GenerateToken(subject string, ttl time.Duration) (string, int64, error)
	VerifyToken(string) (*Claims, error)
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
		if gate == nil || !gate.Enabled() {
			respond(c, gin.H{"enabled": false, "authenticated": true}, nil)
			return
		}

		authenticated := false
		token := extractToken(c)
		if token != "" {
			if _, err := gate.VerifyToken(token); err == nil {
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
		if limiter != nil {
			if err := limiter.check(ip); err != nil {
				c.Error(err)
				return
			}
		}

		input, ok := bindJSON[accessLoginInput](c)
		if !ok {
			return
		}

		if gate == nil {
			respond(c, accessLoginResponse{Success: true}, nil)
			return
		}

		if err := gate.Verify(input.Password); err != nil {
			if limiter != nil {
				limiter.recordFailure(ip)
			}
			c.Error(err)
			return
		}

		if limiter != nil {
			limiter.recordSuccess(ip)
		}

		var token string
		var expiresAt int64
		if gate.Enabled() {
			var genErr error
			token, expiresAt, genErr = gate.GenerateToken("admin", defaultTokenTTL)
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

func accessLogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		setAuthCookie(c, "", -1)
		respond(c, gin.H{"success": true}, nil)
	}
}

func authMiddleware(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if gate == nil || !gate.Enabled() {
			c.Next()
			return
		}

		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "未提供认证令牌，请登录",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		if _, err := gate.VerifyToken(token); err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌无效或已过期，请重新登录",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		c.Next()
	}
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
