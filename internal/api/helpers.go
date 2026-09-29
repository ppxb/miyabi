package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// bindJSON binds the JSON request body into a new instance of T.
// On failure, it attaches a 400 badRequest error to the context and returns false.
func bindJSON[T any](c *gin.Context) (T, bool) {
	var target T
	if err := c.ShouldBindJSON(&target); err != nil {
		c.Error(badRequest(err))
		return target, false
	}
	return target, true
}

// bindQuery binds query string parameters into a new instance of T.
// On failure, it attaches a 400 badRequest error to the context and returns false.
func bindQuery[T any](c *gin.Context) (T, bool) {
	var target T
	if err := c.ShouldBindQuery(&target); err != nil {
		c.Error(badRequest(err))
		return target, false
	}
	return target, true
}

// bindURI binds path parameters into a new instance of T.
// On failure, it attaches a 400 badRequest error to the context and returns false.
func bindURI[T any](c *gin.Context) (T, bool) {
	var target T
	if err := c.ShouldBindUri(&target); err != nil {
		c.Error(badRequest(err))
		return target, false
	}
	return target, true
}

// respond outputs value with HTTP 200 OK, or attaches err to context if non-nil.
func respond(c *gin.Context, value any, err error) {
	respondWithStatus(c, http.StatusOK, value, err)
}

// accepted outputs value with HTTP 202 Accepted, or attaches err to context if non-nil.
func accepted(c *gin.Context, value any, err error) {
	respondWithStatus(c, http.StatusAccepted, value, err)
}

// created outputs value with HTTP 201 Created, or attaches err to context if non-nil.
func created(c *gin.Context, value any, err error) {
	respondWithStatus(c, http.StatusCreated, value, err)
}

// respondWithStatus outputs value with the given status code, or attaches err to context if non-nil.
func respondWithStatus(c *gin.Context, status int, value any, err error) {
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(status, value)
}

// noStore returns a middleware setting Cache-Control: no-store header.
func noStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}
