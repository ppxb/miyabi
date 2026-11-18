package api

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/playback"
)

type PlayManager interface {
	Files(context.Context, int) (playback.PlayFiles, error)
	Start(context.Context, string, string) (playback.Playback, error)
	Stream(context.Context, string, int, string, http.Header) (*http.Response, error)
	Release(string)
}

func playFilesHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		files, err := play.Files(c.Request.Context(), query.MovieID)
		respond(c, files, err)
	}
}

func playStartHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID string `uri:"id" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		result, err := play.Start(c.Request.Context(), uri.ID, c.Request.UserAgent())
		respond(c, result, err)
	}
}

func playStreamHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID       string `uri:"id" binding:"required,uuid"`
			Resource int    `uri:"resource" binding:"min=0"`
		}](c)
		if !ok {
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
		uri, ok := bindURI[struct {
			ID string `uri:"id" binding:"required,uuid"`
		}](c)
		if !ok {
			return
		}
		play.Release(uri.ID)
		respond(c, gin.H{"released": true}, nil)
	}
}
