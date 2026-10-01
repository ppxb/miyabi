package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	lib "github.com/ppxb/miyabi/internal/library"
)

type LibraryManager interface {
	ViewedManager
	Movies(context.Context, int, int) (lib.Page, error)
	Movie(context.Context, int) (lib.MovieDetail, error)
	Preview(context.Context, int, int) (domain.ImageCandidate, error)
	StartScan(context.Context) (domain.TaskInfo, error)
}

func libraryMovieHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID int `uri:"id" binding:"min=1"`
		}](c)
		if !ok {
			return
		}
		detail, err := library.Movie(c.Request.Context(), uri.ID)
		respond(c, detail, err)
	}
}

func libraryPreviewHandler(library LibraryManager, metadata MetadataManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID    int `uri:"id" binding:"min=1"`
			Index int `uri:"index" binding:"min=0"`
		}](c)
		if !ok {
			return
		}
		candidate, err := library.Preview(c.Request.Context(), uri.ID, uri.Index)
		if err != nil {
			c.Error(err)
			return
		}
		media, err := metadata.Image(c.Request.Context(), candidate)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Data(http.StatusOK, media.ContentType, media.Body)
	}
}

type ArtworkReader interface {
	Artwork(string) ([]byte, error)
}

func libraryArtworkHandler(artwork ArtworkReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			Key string `uri:"key" binding:"required,len=64,hexadecimal"`
		}](c)
		if !ok {
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
	Limit int `form:"limit,default=20" binding:"min=1,max=100"`
}

func libraryMoviesHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[libraryPageQuery](c)
		if !ok {
			return
		}
		movies, err := library.Movies(c.Request.Context(), query.Page, query.Limit)
		respond(c, movies, err)
	}
}

func libraryScanHandler(library LibraryManager, emby EmbyManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if emby != nil {
			if err := emby.RetryPending(c.Request.Context()); err != nil {
				c.Error(err)
				return
			}
		}
		task, err := library.StartScan(c.Request.Context())
		accepted(c, task, err)
	}
}
