package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/playback"
)

type PlayManager interface {
	Files(context.Context, int) (playback.PlayFiles, error)
	Start(context.Context, string, string) (playback.Playback, error)
	StartMovie(context.Context, int, string) (playback.Playback, error)
	StartHLS(context.Context, string, string) (playback.Playback, error)
	Stream(context.Context, string, int, string, http.Header) (*http.Response, error)
	Release(string)
	LocateLocalFile(context.Context, string) (string, error)
	PrimaryFile(context.Context, int) (playback.PrimaryFile, error)
	SaveProgress(context.Context, int, string, float64, float64, *domain.PlaybackSettings) error
	History(context.Context, int) (domain.WatchHistoryPage, error)
	RemoveHistory(context.Context, []int) (int, error)
	ClearHistory(context.Context) (int, error)
}

func playQualitiesHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			FileID string `form:"file_id" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		result, err := play.StartHLS(c.Request.Context(), query.FileID, c.Request.UserAgent())
		respond(c, result, err)
	}
}

func playProgressHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, ok := bindJSON[struct {
			MovieID  int                      `json:"movie_id" binding:"min=1"`
			FileID   string                   `json:"file_id" binding:"required,max=128"`
			Position float64                  `json:"position" binding:"gte=0"`
			Duration float64                  `json:"duration" binding:"gt=0"`
			Settings *domain.PlaybackSettings `json:"settings"`
		}](c)
		if !ok {
			return
		}
		respond(c, nil, play.SaveProgress(c.Request.Context(), request.MovieID, request.FileID, request.Position, request.Duration, request.Settings))
	}
}

func historyHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			Page int `form:"page,default=1" binding:"min=1"`
		}](c)
		if !ok {
			return
		}
		page, err := play.History(c.Request.Context(), query.Page)
		respond(c, page, err)
	}
}

func historyRemoveHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, ok := bindJSON[struct {
			IDs []int `json:"ids" binding:"required,min=1,max=100,unique,dive,min=1"`
		}](c)
		if !ok {
			return
		}
		count, err := play.RemoveHistory(c.Request.Context(), request.IDs)
		respond(c, gin.H{"removed": count}, err)
	}
}

func historyClearHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := play.ClearHistory(c.Request.Context())
		respond(c, gin.H{"removed": count}, err)
	}
}

func playStartMovieHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		result, err := play.StartMovie(c.Request.Context(), query.MovieID, c.Request.UserAgent())
		respond(c, result, err)
	}
}

func playFilesHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, ok := bindQuery[struct {
			MovieID int `form:"movie_id" binding:"required,min=1"`
		}](c)
		if !ok {
			return
		}
		files, err := play.Files(c.Request.Context(), query.MovieID)
		respond(c, files, err)
	}
}

func playStartHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID string `uri:"id" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		result, err := play.Start(c.Request.Context(), uri.ID, c.Request.UserAgent())
		respond(c, result, err)
	}
}

func playStreamHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID       string `uri:"id" binding:"required,uuid"`
			Resource int    `uri:"resource" binding:"min=0"`
		}](c)
		if !ok {
			return
		}
		response, err := play.Stream(c.Request.Context(), uri.ID, uri.Resource, c.Request.Method, c.Request.Header)
		if err != nil {
			c.Error(err)
			return
		}
		defer response.Body.Close()
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Location"} {
			if value := response.Header.Get(name); value != "" {
				c.Header(name, value)
			}
		}
		c.Status(response.StatusCode)
		if c.Request.Method != http.MethodHead {
			if _, err := io.Copy(c.Writer, response.Body); err != nil {
				c.Error(err)
			}
		}
	}
}

func playReleaseHandler(play PlayManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[struct {
			ID string `uri:"id" binding:"required,uuid"`
		}](c)
		if !ok {
			return
		}
		play.Release(uri.ID)
		respond(c, gin.H{"released": true}, nil)
	}
}

// localStreamHandler serves an indexed local video from disk with Range support,
// or redirects a local .strm to the URL it wraps. It accepts the STRM token so an
// external player (MPV) without a session cookie can use it, and a signed-in
// cookie for the built-in player. Only paths inside a configured local source
// directory (validated by LocateLocalFile) are served.
func localStreamHandler(play PlayManager, token string, gate AccessGate) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strmAuthorized(c, token, gate) {
			c.Error(domain.E(domain.KindUnauthorized, "无效的播放令牌", nil))
			return
		}
		uri, ok := bindURI[struct {
			FileID string `uri:"fileID" binding:"required,max=128"`
		}](c)
		if !ok {
			return
		}
		path, err := play.LocateLocalFile(c.Request.Context(), uri.FileID)
		if err != nil {
			c.Error(err)
			return
		}
		if strings.EqualFold(filepath.Ext(path), ".strm") {
			body, err := os.ReadFile(path)
			if err != nil {
				c.Error(err)
				return
			}
			target := firstNonBlankLine(string(body))
			parsed, parseErr := url.Parse(target)
			if target == "" || parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				c.Error(domain.E(domain.KindInvalid, "本地 STRM 内容不是可播放的链接", nil))
				return
			}
			c.Redirect(http.StatusFound, target)
			return
		}
		file, err := os.Open(path)
		if err != nil {
			c.Error(err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			c.Error(err)
			return
		}
		http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), file)
	}
}

func firstNonBlankLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
