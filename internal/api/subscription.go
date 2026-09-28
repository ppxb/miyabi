package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/monitor"
)

type SubscriptionManager interface {
	List(ctx context.Context, kind string, page int, limit int) ([]monitor.Item, error)
	Targets(ctx context.Context, kind string) ([]monitor.TargetItem, error)
	AddMovie(ctx context.Context, movieID string, opts monitor.AddMovieOptions) (monitor.Item, error)
	AddActor(ctx context.Context, actorID string, opts monitor.AddActorOptions) (monitor.Item, error)
	Update(ctx context.Context, id int, opts monitor.UpdateOptions) (monitor.Item, error)
	Remove(ctx context.Context, id int) error
	EnqueueSingle(ctx context.Context, id int) (monitor.Item, error)
	EnqueueBatch(ctx context.Context, req monitor.BatchEnqueueRequest) (int, error)
	ActorFeed(ctx context.Context, actorID int, page int, limit int) ([]monitor.Item, error)
	Config(ctx context.Context) (monitor.Config, error)
	UpdateConfig(ctx context.Context, cfg monitor.Config) error
}

type subscriptionListQuery struct {
	Kind  string `form:"kind" binding:"omitempty,oneof=movie actor"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=50" binding:"min=1,max=100"`
}

type subscriptionTargetsQuery struct {
	Kind string `form:"kind" binding:"omitempty,oneof=movie actor"`
}

type subscriptionCreateInput struct {
	Kind         string `json:"kind" binding:"required,oneof=movie actor"`
	TargetID     string `json:"target_id" binding:"required"`
	Title        string `json:"title"`
	Cover        string `json:"cover"`
	AutoDownload *bool  `json:"auto_download"`
	Zone         string `json:"zone"`
}

type subscriptionUpdateInput struct {
	AutoDownload *bool   `json:"auto_download"`
	Zone         *string `json:"zone"`
	Status       *string `json:"status" binding:"omitempty,oneof=active paused"`
}

type subscriptionEnqueueBatchInput struct {
	IDs []int `json:"ids"`
	All bool  `json:"all"`
}

type subscriptionURI struct {
	ID int `uri:"id" binding:"required,min=1"`
}

type optionalSubscriptionURI struct {
	ID int `uri:"id" binding:"omitempty,min=1"`
}

func subscriptionTargetsHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[subscriptionTargetsQuery](c)
		if !ok {
			return
		}
		items, err := mgr.Targets(c.Request.Context(), query.Kind)
		respond(c, items, err)
	}
}

func subscriptionListHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[subscriptionListQuery](c)
		if !ok {
			return
		}
		items, err := mgr.List(c.Request.Context(), query.Kind, query.Page, query.Limit)
		respond(c, items, err)
	}
}

func subscriptionCreateHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[subscriptionCreateInput](c)
		if !ok {
			return
		}
		var item monitor.Item
		var err error
		if input.Kind == "actor" {
			item, err = mgr.AddActor(c.Request.Context(), input.TargetID, monitor.AddActorOptions{
				Title:        input.Title,
				Cover:        input.Cover,
				AutoDownload: input.AutoDownload,
				Zone:         input.Zone,
			})
		} else {
			item, err = mgr.AddMovie(c.Request.Context(), input.TargetID, monitor.AddMovieOptions{
				AutoDownload: input.AutoDownload,
				Zone:         input.Zone,
			})
		}
		created(c, item, err)
	}
}

func subscriptionUpdateHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subscriptionURI](c)
		if !ok {
			return
		}
		input, ok := bindJSON[subscriptionUpdateInput](c)
		if !ok {
			return
		}
		item, err := mgr.Update(c.Request.Context(), uri.ID, monitor.UpdateOptions{
			AutoDownload: input.AutoDownload,
			Zone:         input.Zone,
			Status:       input.Status,
		})
		respond(c, item, err)
	}
}

func subscriptionRemoveHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subscriptionURI](c)
		if !ok {
			return
		}
		err := mgr.Remove(c.Request.Context(), uri.ID)
		respond(c, nil, err)
	}
}

func subscriptionEnqueueSingleHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subscriptionURI](c)
		if !ok {
			return
		}
		item, err := mgr.EnqueueSingle(c.Request.Context(), uri.ID)
		respond(c, item, err)
	}
}

func subscriptionEnqueueBatchHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		input, ok := bindJSON[subscriptionEnqueueBatchInput](c)
		if !ok {
			return
		}
		taskID, err := mgr.EnqueueBatch(c.Request.Context(), monitor.BatchEnqueueRequest{
			IDs: input.IDs,
			All: input.All,
		})
		if err != nil {
			respond(c, nil, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"task_id": taskID})
	}
}

func subscriptionActorFeedHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, _ := bindURI[optionalSubscriptionURI](c)
		query, ok := bindQuery[subscriptionListQuery](c)
		if !ok {
			return
		}
		items, err := mgr.ActorFeed(c.Request.Context(), uri.ID, query.Page, query.Limit)
		respond(c, items, err)
	}
}

func subscriptionSettingsGetHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, err := mgr.Config(c.Request.Context())
		respond(c, cfg, err)
	}
}

func subscriptionSettingsUpdateHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg, ok := bindJSON[monitor.Config](c)
		if !ok {
			return
		}
		err := mgr.UpdateConfig(c.Request.Context(), cfg)
		respond(c, cfg, err)
	}
}
