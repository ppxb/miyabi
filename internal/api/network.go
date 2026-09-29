package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/network"
	"github.com/ppxb/miyabi/internal/netx"
)

type NetworkManager interface {
	Config() netx.ProxyConfig
	UpdateNetwork(context.Context, netx.ProxyConfig) error
	TestNetwork(context.Context, netx.ProxyConfig) (network.TestResult, error)
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

// networkTestHandler probes with the configuration in the request body, or
// with the saved configuration when the body is empty.
func networkTestHandler(network NetworkManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		config := network.Config()
		if c.Request.ContentLength != 0 {
			bodyConfig, ok := bindJSON[netx.ProxyConfig](c)
			if !ok {
				return
			}
			config = bodyConfig
		}
		result, err := network.TestNetwork(c.Request.Context(), config)
		respond(c, result, err)
	}
}
