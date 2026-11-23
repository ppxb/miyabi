package api

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/ppxb/miyabi/internal/subtitle"
)

// SubtitleManager lets the user search, preview, save and remove the
// subtitles of a library movie.
type SubtitleManager interface {
	List(ctx context.Context, movieID int) ([]subtitle.Track, error)
	Search(ctx context.Context, movieID int) ([]subtitle.Offer, error)
	Preview(ctx context.Context, movieID int, offer subtitle.Offer) (subtitle.Preview, error)
	Save(ctx context.Context, movieID int, offer subtitle.Offer) (subtitle.Track, error)
	Delete(ctx context.Context, movieID, trackID int) error
	Cues(ctx context.Context, movieID, trackID int) ([]subtitle.TimedCue, error)
	Text(ctx context.Context, trackID int) (subtitle.SubtitleText, error)
}

type subtitleTrackURI struct {
	ID         int `uri:"id" binding:"min=1"`
	SubtitleID int `uri:"subtitleID" binding:"min=1"`
}

type subtitleMovieURI struct {
	ID int `uri:"id" binding:"min=1"`
}

// subtitleOfferInput is an offer returned by search; the URL is downloaded
// through the SSRF-safe client.
type subtitleOfferInput struct {
	Provider string `json:"provider" binding:"max=64"`
	Name     string `json:"name" binding:"max=512"`
	URL      string `json:"url" binding:"required,url,max=2048"`
	Format   string `json:"format" binding:"required,oneof=srt ass ssa vtt"`
	Language string `json:"language" binding:"max=16"`
	Version  string `json:"version" binding:"max=32"`
}

func (input subtitleOfferInput) offer() subtitle.Offer {
	return subtitle.Offer(input)
}

func subtitleListHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleMovieURI](c)
		if !ok {
			return
		}
		tracks, err := subtitles.List(c.Request.Context(), uri.ID)
		respond(c, tracks, err)
	}
}

func subtitleSearchHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleMovieURI](c)
		if !ok {
			return
		}
		offers, err := subtitles.Search(c.Request.Context(), uri.ID)
		respond(c, offers, err)
	}
}

func subtitlePreviewHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleMovieURI](c)
		if !ok {
			return
		}
		input, ok := bindJSON[subtitleOfferInput](c)
		if !ok {
			return
		}
		preview, err := subtitles.Preview(c.Request.Context(), uri.ID, input.offer())
		respond(c, preview, err)
	}
}

func subtitleSaveHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleMovieURI](c)
		if !ok {
			return
		}
		input, ok := bindJSON[subtitleOfferInput](c)
		if !ok {
			return
		}
		track, err := subtitles.Save(c.Request.Context(), uri.ID, input.offer())
		created(c, track, err)
	}
}

func subtitleDeleteHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleTrackURI](c)
		if !ok {
			return
		}
		err := subtitles.Delete(c.Request.Context(), uri.ID, uri.SubtitleID)
		respond(c, gin.H{"deleted": true}, err)
	}
}

// subtitleCuesHandler serves a track as player cues, so every format the
// library holds plays in the browser without converting files.
func subtitleCuesHandler(subtitles SubtitleManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		uri, ok := bindURI[subtitleTrackURI](c)
		if !ok {
			return
		}
		cues, err := subtitles.Cues(c.Request.Context(), uri.ID, uri.SubtitleID)
		respond(c, cues, err)
	}
}
