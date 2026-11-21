package api

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/playback"
)

type PlayManager interface {
	Files(context.Context, int) (playback.PlayFiles, error)
	Start(context.Context, string, string) (playback.Playback, error)
	StartMovie(context.Context, int, string) (playback.Playback, error)
	StartHLS(context.Context, string, string) (playback.Playback, error)
	Stream(context.Context, string, int, string, http.Header) (*http.Response, error)
	Release(string)
	SaveProgress(context.Context, int, string, float64, float64, *domain.PlaybackSettings) error
	History(context.Context, int) (domain.WatchHistoryPage, error)
	RemoveHistory(context.Context, []int) (int, error)
	ClearHistory(context.Context) (int, error)
}

func playQualitiesHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			FileID string `form:"file_id" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		result, err := play.StartHLS(c.Request.Context(), query.FileID, c.Request.UserAgent())
		respond(c, result, err)
	}
}

func playProgressHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, ok := bindJSON[struct {
			MovieID  int                      `json:"movie_id" binding:"min=1"`
			FileID   string                   `json:"file_id" binding:"required,max=128"`
			Position float64                  `json:"position" binding:"gte=0"`
			Duration float64                  `json:"duration" binding:"gt=0"`
			Settings *domain.PlaybackSettings `json:"settings"`
		}](c)
		if !ok {
			return
		}
		respond(c, nil, play.SaveProgress(c.Request.Context(), request.MovieID, request.FileID, request.Position, request.Duration, request.Settings))
	}
}

func historyHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			Page int `form:"page,default=1" binding:"min=1"`
		}](c)
		if !ok {
			return
		}
		page, err := play.History(c.Request.Context(), query.Page)
		respond(c, page, err)
	}
}

func historyRemoveHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, ok := bindJSON[struct {
			IDs []int `json:"ids" binding:"required,min=1,max=100,unique,dive,min=1"`
		}](c)
		if !ok {
			return
		}
		count, err := play.RemoveHistory(c.Request.Context(), request.IDs)
		respond(c, gin.H{"removed": count}, err)
	}
}

func historyClearHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := play.ClearHistory(c.Request.Context())
		respond(c, gin.H{"removed": count}, err)
	}
}

func playStartMovieHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		result, err := play.StartMovie(c.Request.Context(), query.MovieID, c.Request.UserAgent())
		respond(c, result, err)
	}
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
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Location"} {
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
