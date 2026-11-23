package api

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// The MPV bridge scripts are served so the browser can download them and the
// user can run them once on the playback machine.

//go:embed mpvbridge/install-mpv-bridge.ps1
var mpvBridgeInstall []byte

//go:embed mpvbridge/uninstall-mpv-bridge.ps1
var mpvBridgeUninstall []byte

// mpvBridgeScriptHandler downloads one bridge script as an attachment.
func mpvBridgeScriptHandler(body []byte, filename string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
		c.Data(http.StatusOK, "text/plain; charset=utf-8", body)
	}
}
