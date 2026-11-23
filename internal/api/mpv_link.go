package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
)

// requestBase reconstructs the externally reachable base URL of this Miyabi
// instance from the incoming request, so the mpv deep link points at a host the
// playback machine can reach.
func requestBase(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

func withToken(base, token string) string {
	if token == "" {
		return base
	}
	return base + "?token=" + url.QueryEscape(token)
}

// mpvLinkHandler builds a miyabi-mpv:// deep link for a movie: the video stream
// URL and the subtitle file URL, both usable by an external player without a
// session cookie. When access is password-protected, the fixed STRM token
// authorizes them, mirroring how exported .strm files work.
func mpvLinkHandler(play PlayManager, subtitles SubtitleManager, token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		files, err := play.Files(c.Request.Context(), query.MovieID)
		if err != nil {
			respond(c, nil, err)
			return
		}
		if len(files.Files) == 0 {
			respond(c, nil, domain.E(domain.KindNotFound, "当前挂载目录中未找到可播放文件", nil))
			return
		}
		base := requestBase(c)
		videoURL := withToken(base+"/api/strm/play/"+files.Files[0].ID, token)
		link := "miyabi-mpv://play?u=" + url.QueryEscape(videoURL)

		tracks, err := subtitles.List(c.Request.Context(), query.MovieID)
		if err != nil {
			respond(c, nil, err)
			return
		}
		if len(tracks) > 0 {
			subURL := withToken(base+"/api/subtitle-file/"+strconv.Itoa(tracks[0].ID), token)
			link += "&s=" + url.QueryEscape(subURL)
		}
		respond(c, gin.H{"url": link}, nil)
	}
}

// subtitleFileHandler serves a saved subtitle decoded to UTF-8 for external
// players. It skips the cookie auth because players cannot sign in; it uses the
// same token/session policy as the STRM relay instead.
func subtitleFileHandler(subtitles SubtitleManager, token string, gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			TrackID int `uri:"trackID" binding:"min=1"`
		}](c)
		if !ok {
			return
		}
		if !strmAuthorized(c, token, gate) {
			c.Error(domain.E(domain.KindUnauthorized, "无效的播放令牌", nil))
			return
		}
		text, err := subtitles.Text(c.Request.Context(), uri.TrackID)
		if err != nil {
			c.Error(err)
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Content-Disposition", `inline; filename="`+text.Name+`"`)
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusOK, text.Text)
	}
}
