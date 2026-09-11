package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/service"
)

func libraryHistoryHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var query struct {
			Page int `form:"page,default=1" binding:"min=1,max=100000000"`
		}
		if err := c.ShouldBindQuery(&query); err != nil {
			c.Error(BadRequest(err))
			return
		}
		page, err := library.WatchHistory(c.Request.Context(), query.Page)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

func libraryHistoryProgressHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var uri struct {
			ID int `uri:"id" binding:"min=1"`
		}
		if err := c.ShouldBindUri(&uri); err != nil {
			c.Error(BadRequest(err))
			return
		}
		var progress service.WatchProgress
		if err := c.ShouldBindJSON(&progress); err != nil {
			c.Error(BadRequest(err))
			return
		}
		if err := library.SaveWatchProgress(c.Request.Context(), uri.ID, progress); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, nil)
	}
}

func libraryHistoryRemoveHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			service.WatchHistoryScope
			IDs []int `json:"ids" binding:"required,min=1,max=100,unique,dive,min=1"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.Error(BadRequest(err))
			return
		}
		count, err := library.RemoveWatchHistory(c.Request.Context(), request.WatchHistoryScope, request.IDs)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": count})
	}
}

func libraryHistoryClearHandler(library LibraryManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var scope service.WatchHistoryScope
		if err := c.ShouldBindQuery(&scope); err != nil {
			c.Error(BadRequest(err))
			return
		}
		count, err := library.ClearWatchHistory(c.Request.Context(), scope)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": count})
	}
}
