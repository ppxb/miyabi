package domain

import "time"

// LibraryFile is an indexed video file inside a mounted media source.
type LibraryFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// WatchResume is the last saved playback position of a movie.
type WatchResume struct {
	FileID   string  `json:"file_id"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
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
