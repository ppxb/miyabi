package api

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type PlayManager interface {
	Files(context.Context, string) (service.PlayFiles, error)
	Start(context.Context, string, bool) (service.Playback, error)
	Stream(context.Context, string, int, string, http.Header) (*http.Response, error)
	Release(string)
}

func playFilesHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query struct {
			Code string `form:"code" binding:"required,max=200"`
		}
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		files, err := play.Files(c.Request.Context(), query.Code)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, files)
	}
}

func playStartHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID string `uri:"id" binding:"required,numeric,max=30"`
		}
		var query struct {
			Mode string `form:"mode,default=original" binding:"oneof=original hls"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		playback, err := play.Start(c.Request.Context(), uri.ID, query.Mode == "hls")
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, playback)
	}
}

func playStreamHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID       string `uri:"id" binding:"required,uuid"`
			Resource int    `uri:"resource" binding:"min=0"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		response, err := play.Stream(c.Request.Context(), uri.ID, uri.Resource, c.Request.Method, c.Request.Header)
		if err != nil {
			c.Error(err)
			return
		}
		defer response.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
			if value := response.Header.Get(name); value != "" {
				c.Header(name, value)
			}
		}
		c.Status(response.StatusCode)
		if c.Request.Method != http.MethodHead {
			if _, err := io.Copy(c.Writer, response.Body); err != nil {
				c.Error(err)
			}
		}
	}
}

func playReleaseHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID string `uri:"id" binding:"required,uuid"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		play.Release(uri.ID)
		c.JSON(http.StatusOK, gin.H{"released": true})
	}
}
