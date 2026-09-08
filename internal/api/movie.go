package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

type LibraryManager interface {
	Movies(context.Context, int, int) (service.LibraryPage, error)
	Files(context.Context, int, bool, int, int) (service.LibraryFilePage, error)
	StartScan(context.Context) (service.TaskInfo, error)
}

type ArtworkReader interface {
	Artwork(string) ([]byte, error)
}

func libraryArtworkHandler(artwork ArtworkReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			Key string `uri:"key" binding:"required,len=64,hexadecimal"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		body, err := artwork.Artwork(uri.Key)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, "image/jpeg", body)
	}
}

type libraryPageQuery struct {
	Page  int `form:"page,default=1" binding:"min=1"`
	Limit int `form:"limit,default=24" binding:"min=1,max=100"`
}

type libraryFilesQuery struct {
	libraryPageQuery
	MovieID   int  `form:"movie_id" binding:"omitempty,min=1"`
	Unmatched bool `form:"unmatched"`
}

func libraryMoviesHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query libraryPageQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		movies, err := library.Movies(c.Request.Context(), query.Page, query.Limit)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, movies)
	}
}

func libraryFilesHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query libraryFilesQuery
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		files, err := library.Files(c.Request.Context(), query.MovieID, query.Unmatched, query.Page, query.Limit)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, files)
	}
}

func libraryScanHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		task, err := library.StartScan(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}
