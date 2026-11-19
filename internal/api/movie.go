package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	lib "github.com/ppxb/miyabi/internal/library"
)

type LibraryManager interface {
	ViewedManager
	Movies(context.Context, int, int) (lib.Page, error)
	BrowseMovies(context.Context, lib.BrowseFilter) (lib.Page, error)
	FilterOptions(context.Context) (lib.FilterOptions, error)
	TopRated(context.Context, int) ([]lib.Movie, error)
	Movie(context.Context, int) (lib.MovieDetail, error)
	Preview(context.Context, int, int) (domain.ImageCandidate, error)
	StartScan(context.Context) (domain.TaskInfo, error)
	StartRebuild(context.Context) (domain.TaskInfo, error)
	StartIncrementalSync(context.Context) (int, error)
	PendingCount(context.Context) (int, error)
	RescrapeMovie(context.Context, int, string) (domain.TaskInfo, error)
}

func libraryMovieScrapeHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID int `uri:"id" binding:"min=1"`
		}](c)
		if !ok {
			return
		}
		input, ok := bindJSON[struct {
			Code string `json:"code" binding:"max=120"`
		}](c)
		if !ok {
			return
		}
		job, err := library.RescrapeMovie(c.Request.Context(), uri.ID, input.Code)
		accepted(c, job, err)
	}
}

func libraryRebuildHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		job, err := library.StartRebuild(c.Request.Context())
		accepted(c, job, err)
	}
}

func librarySyncHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		started, err := library.StartIncrementalSync(c.Request.Context())
		respond(c, gin.H{"started": started}, err)
	}
}

func libraryPendingHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := library.PendingCount(c.Request.Context())
		respond(c, gin.H{"count": count}, err)
	}
}

func libraryTopRatedHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			Limit int `form:"limit,default=12" binding:"min=1,max=50"`
		}](c)
		if !ok {
			return
		}
		movies, err := library.TopRated(c.Request.Context(), query.Limit)
		respond(c, movies, err)
	}
}

func libraryBrowseHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, err := queryInt(c, "page", 1, 1, 1<<31-1)
		if err != nil {
			c.Error(badRequest(err))
			return
		}
		limit, err := queryInt(c, "limit", 20, 1, 100)
		if err != nil {
			c.Error(badRequest(err))
			return
		}
		filter := lib.BrowseFilter{
			Query:       c.Query("q"),
			Sort:        c.DefaultQuery("sort", "recent"),
			Source:      c.Query("source"),
			HighRated:   c.Query("high") == "true" || c.Query("high") == "1",
			TagNames:    c.QueryArray("tag"),
			ActorNames:  c.QueryArray("actor"),
			MakerNames:  c.QueryArray("maker"),
			SeriesNames: c.QueryArray("series"),
			Page:        page,
			Limit:       limit,
		}
		movies, err := library.BrowseMovies(c.Request.Context(), filter)
		respond(c, movies, err)
	}
}

func libraryFilterOptionsHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := library.FilterOptions(c.Request.Context())
		respond(c, options, err)
	}
}

func queryInt(c *gin.Context, key string, fallback, min, max int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s 参数无效", key)
	}
	return value, nil
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
