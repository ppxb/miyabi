package api

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

// STRMRelay resolves the fixed URLs written into exported .strm files.
type STRMRelay interface {
	StreamURL(ctx context.Context, fileID, userAgent string) (string, error)
	Probe(context.Context, string, http.Header) (*http.Response, error)
}

// strmStreamHandler redirects media servers to a fresh 115 stream URL.
// HEAD requests are answered from the CDN so clients can probe the stream
// without following the redirect.
func strmStreamHandler(relay STRMRelay, token string, gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			FileID string `uri:"fileID" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		if !strmAuthorized(c, token, gate) {
			c.Error(domain.E(domain.KindUnauthorized, "无效的播放令牌", nil))
			return
		}
		streamURL, err := relay.StreamURL(c.Request.Context(), uri.FileID, c.Request.UserAgent())
		if err != nil {
			c.Error(err)
			return
		}
		if c.Request.Method == http.MethodHead {
			if response, err := relay.Probe(c.Request.Context(), streamURL, c.Request.Header); err == nil {
				defer response.Body.Close()
				for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
					if value := response.Header.Get(name); value != "" {
						c.Header(name, value)
					}
				}
				if c.Writer.Header().Get("Content-Type") == "" || c.Writer.Header().Get("Content-Type") == "application/octet-stream" {
					c.Header("Content-Type", "video/mp4")
				}
				c.Status(response.StatusCode)
				return
			}
		}
		c.Redirect(http.StatusFound, streamURL)
	}
}

// Media servers cannot sign in, so the configured STRM token authorizes a
// request on its own. A signed-in session is accepted as well; without a
// token or access password the relay is open.
func strmAuthorized(c *gin.Context, token string, gate AccessGate) bool {
	if token != "" && subtle.ConstantTimeCompare([]byte(c.Query("token")), []byte(token)) == 1 {
		return true
	}
	if gate != nil && gate.Enabled() {
		session := extractToken(c)
		if session == "" {
			return false
		}
		_, err := gate.VerifyToken(session)
		return err == nil
	}
	return token == ""
}
