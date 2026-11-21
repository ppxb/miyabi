package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
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
}

// Value lets ent store PlaybackSettings in a JSON column via database/sql.
func (s PlaybackSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan lets ent read a JSON column back into PlaybackSettings.
func (s *PlaybackSettings) Scan(value any) error {
	if value == nil {
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type %T for PlaybackSettings", value)
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, s)
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
