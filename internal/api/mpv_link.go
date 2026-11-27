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
const mpvUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func mpvLinkHandler(play PlayManager, relay STRMRelay, subtitles SubtitleManager, token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		file, err := play.PrimaryFile(c.Request.Context(), query.MovieID)
		if err != nil {
			respond(c, nil, err)
			return
		}
		if file.FileID == "" {
			respond(c, nil, domain.E(domain.KindNotFound, "当前挂载目录中未找到可播放文件", nil))
			return
		}
		base := requestBase(c)
		relayURL := ""
		if !file.Local {
			relayURL = withToken(base+"/api/strm/play/"+file.FileID, token)
		}
		var videoURL string
		if file.Local {
			// Local files are served from disk.
			videoURL = withToken(base+"/api/play-local/"+file.FileID, token)
		} else if file.Provider == "guangya" {
			// Guangya VOD URLs are temporary direct mp4 links; give MPV the
			// signed URL instead of adding a token-protected relay hop.
			if direct, err := relay.StreamURL(c.Request.Context(), file.FileID, mpvUserAgent); err == nil && direct != "" {
				videoURL = direct
			} else {
				videoURL = withToken(base+"/api/strm/play/"+file.FileID, token)
			}
		} else {
			// 115 keeps the stable relay so its short-lived signed URL is
			// resolved by the server at playback time.
			videoURL = withToken(base+"/api/strm/play/"+file.FileID, token)
		}
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
		respond(c, gin.H{"url": link, "video_url": videoURL, "relay_url": relayURL}, nil)
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
