package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type AccessGate interface {
	Enabled() bool
	Verify(string) error
}

type accessLoginInput struct {
	Password string `json:"password" binding:"required"`
}

func accessConfigHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"enabled": gate.Enabled()})
	}
}

func accessLoginHandler(gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input accessLoginInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := gate.Verify(input.Password); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
