package openplatform

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"nova-erp/internal/config"
	"nova-erp/internal/middleware"
	"nova-erp/internal/module"
)

func Module(cfg config.OpenPlatformConfig) module.Module {
	h := handler{bridge: bridge{
		url: strings.TrimRight(cfg.BridgeURL, "/"),
		secret: cfg.BridgeSecret,
		client: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}}
	return module.Module{
		Namespace: "openplatform",
		Register: func(rg *gin.RouterGroup) {
			g := rg.Group("/open-platform", middleware.Auth())
			g.POST("/device/inspect", h.inspect)
			g.POST("/device/decision", h.decide)
			g.GET("/grants", h.list)
			g.POST("/grants/:id/revoke", h.revoke)
		},
	}
}
