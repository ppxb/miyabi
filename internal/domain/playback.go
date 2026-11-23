package domain

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// LibraryFile is an indexed video file inside a mounted media source.
type LibraryFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// PlaybackSettings holds per-movie playback preferences in one extensible blob.
// New preferences (subtitles, audio tracks, ...) become new fields here without
// a schema change, since it is persisted as JSON on the watch-history record.
type PlaybackSettings struct {
	Rate    float64 `json:"rate,omitempty"`
	Quality string  `json:"quality,omitempty"`
	// Subtitle is the chosen subtitle track id, or "off".
	Subtitle string `json:"subtitle,omitempty"`
	// SubtitleOffset shifts subtitle cues in seconds; zero is a real choice,
	// so a nil pointer means "not mentioned".
	SubtitleOffset *float64 `json:"subtitle_offset,omitempty"`
}

// Value lets an upsert pass PlaybackSettings to the SQL driver as JSON. Reads
// go through ent's JSON field decoding; implementing sql.Scanner here would make
// the ent generator emit broken scan code for the field.
func (s PlaybackSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// WatchResume is the last saved playback position of a movie together with its
// saved playback preferences, so reopening restores speed, rendition and so on.
type WatchResume struct {
	FileID   string            `json:"file_id"`
	Position float64           `json:"position"`
	Duration float64           `json:"duration"`
	Settings *PlaybackSettings `json:"settings,omitempty"`
}

type WatchHistoryItem struct {
	MovieID     int       `json:"movie_id"`
	Code        string    `json:"code"`
	Title       string    `json:"title"`
	Cover       *string   `json:"cover,omitempty"`
	Poster      *string   `json:"poster,omitempty"`
	Rating      float64   `json:"rating"`
	Actors      []Actor   `json:"actors"`
	Source      string    `json:"source,omitempty"`
	HasSubtitle bool      `json:"has_subtitle"`
	WatchedAt   time.Time `json:"watched_at"`
	Position    float64   `json:"position"`
	Duration    float64   `json:"duration"`
}

type WatchHistoryPage struct {
	Items   []WatchHistoryItem `json:"items"`
	Total   int                `json:"total"`
	Page    int                `json:"page"`
	HasMore bool               `json:"has_more"`
}
