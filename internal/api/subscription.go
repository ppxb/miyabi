package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/monitor"
)

type SubscriptionManager interface {
	List(ctx context.Context, kind string, page int, limit int) ([]monitor.Item, error)
	Targets(ctx context.Context, kind string) ([]monitor.TargetItem, error)
	AddActor(ctx context.Context, actorKey string) (monitor.Item, error)
	Update(ctx context.Context, id int, opts monitor.UpdateOptions) (monitor.Item, error)
	Remove(ctx context.Context, id int) error
	ActorFeed(ctx context.Context, actorID int, page int, limit int) ([]monitor.Item, error)
	Config(ctx context.Context) (monitor.Config, error)
	UpdateConfig(ctx context.Context, cfg monitor.Config) (monitor.Config, error)
}

type subscriptionListQuery struct {
	Kind  string `form:"kind" binding:"omitempty,oneof=movie actor"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=50" binding:"min=1,max=100"`
}

type subscriptionFeedQuery struct {
	Page  int `form:"page,default=1" binding:"min=1"`
	Limit int `form:"limit,default=50" binding:"min=1,max=100"`
}

type subscriptionTargetsQuery struct {
	Kind string `form:"kind" binding:"omitempty,oneof=movie actor"`
}

type subscriptionCreateInput struct {
	// Only library actors can be followed, by actor key; movie subscriptions
	// come from actor checks.
	Kind     string `json:"kind" binding:"required,oneof=actor"`
	TargetID string `json:"target_id" binding:"required"`
}

type subscriptionUpdateInput struct {
	Status *string `json:"status" binding:"omitempty,oneof=active paused"`
}

type subscriptionURI struct {
	ID int `uri:"id" binding:"required,min=1"`
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
		item, err := mgr.AddActor(c.Request.Context(), input.TargetID)
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
		item, err := mgr.Update(c.Request.Context(), uri.ID, monitor.UpdateOptions{Status: input.Status})
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

func subscriptionActorFeedHandler(mgr SubscriptionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		var actorID int
		if c.Param("id") != "" {
			uri, ok := bindURI[subscriptionURI](c)
			if !ok {
				return
			}
			actorID = uri.ID
		}
		query, ok := bindQuery[subscriptionFeedQuery](c)
		if !ok {
			return
		}
		items, err := mgr.ActorFeed(c.Request.Context(), actorID, query.Page, query.Limit)
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
		saved, err := mgr.UpdateConfig(c.Request.Context(), cfg)
		respond(c, saved, err)
	}
}
