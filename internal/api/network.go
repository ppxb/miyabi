package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/netx"
)

type NetworkManager interface {
	Config() netx.ProxyConfig
	UpdateNetwork(context.Context, netx.ProxyConfig) error
}

func networkHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		respond(c, network.Config(), nil)
	}
}

func networkUpdateHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config, ok := bindJSON[netx.ProxyConfig](c)
		if !ok {
			return
		}
		if err := network.UpdateNetwork(c.Request.Context(), config); err != nil {
			c.Error(err)
			return
		}
		respond(c, network.Config(), nil)
	}
}
