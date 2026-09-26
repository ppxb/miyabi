package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	lib "github.com/ppxb/miyabi/internal/library"
	"github.com/ppxb/miyabi/internal/library/scan"
	"github.com/ppxb/miyabi/internal/tasks"
)

type LibraryManager interface {
	ViewedManager
	Movies(context.Context, int, int) (lib.Page, error)
	StartScan(context.Context) (tasks.TaskInfo, error)
	ScanLocal(context.Context, string) (*scan.LocalScanResult, error)
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

func libraryScanHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		task, err := library.StartScan(c.Request.Context())
		accepted(c, task, err)
	}
}

type localScanRequest struct {
	Path string `json:"path"`
}

func libraryLocalScanHandler(library LibraryManager, defaultDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, _ := bindJSON[localScanRequest](c)
		scanPath := req.Path
		if scanPath == "" {
			scanPath = defaultDir
		}
		if scanPath == "" {
			c.Error(domain.E(domain.KindInvalid, "未指定扫描目录且未配置 Emby 目录", nil))
			return
		}
		res, err := library.ScanLocal(c.Request.Context(), scanPath)
		respond(c, res, err)
	}
}
