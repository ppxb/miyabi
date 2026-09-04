package javdb

import (
	"fmt"
	"time"
)

const (
	appVersion       = "1.9.28"
	appVersionNumber = "10928"
	userAgent        = "Dart/3.4 (dart:io)"
	defaultLanguage  = "zh-TW"
	defaultTimeout   = 20 * time.Second
	defaultRate      = 2.0
	defaultBurst     = 1
)

// Options configures the anonymous JavDB App API client.
type Options struct {
	CachedHost        string
	DeviceUUID        string
	Proxy             string
	Timeout           time.Duration
	RequestsPerSecond float64
	Burst             int
}

// RouteStatus describes the currently selected API route.
type RouteStatus struct {
	Host    string
	Latency time.Duration
}

// Zone is a JavDB movie section.
type Zone string

const (
	ZoneAll        Zone = "all"
	ZoneCensored   Zone = "censored"
	ZoneUncensored Zone = "uncensored"
	ZoneWestern    Zone = "western"
	ZoneFC2        Zone = "fc2"
)

// SearchOptions controls a movie search request.
type SearchOptions struct {
	Zone     Zone
	Sort     string
	FilterBy string
	Page     int
	Limit    int
}

// BrowseOptions controls a category browse request.
type BrowseOptions struct {
	Zone   Zone
	Main   []string
	TagIDs []string
	Year   string
	Month  string
	Sort   string
	Order  string
	Page   int
	Limit  int
}

// PreviewImage is one image returned for a movie preview gallery.
type PreviewImage struct {
	Thumbnail string `json:"thumbnail"`
	Original  string `json:"original"`
}

// Actor is an actor attached to a movie.
type Actor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameZHT string `json:"name_zht"`
	Gender  string `json:"gender"`
	Avatar  string `json:"avatar"`
}

// Tag is a JavDB content tag.
type Tag struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	NameZHT    string `json:"name_zht"`
	CategoryID string `json:"category_id"`
}

// TagCategory groups the bilingual tag taxonomy returned by JavDB.
type TagCategory struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameZHT string `json:"name_zht"`
	Tags    []Tag  `json:"tags"`
}

type Series struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Maker struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Director struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Movie is the stable movie model exposed to Miyabi's service layer.
type Movie struct {
	ID            string         `json:"id"`
	Code          string         `json:"code"`
	Title         string         `json:"title"`
	OriginTitle   string         `json:"origin_title"`
	ReleaseDate   string         `json:"release_date"`
	Duration      int            `json:"duration"`
	Rating        float64        `json:"rating"`
	Thumbnail     string         `json:"thumbnail"`
	Cover         string         `json:"cover"`
	PreviewImages []PreviewImage `json:"preview_images"`
	PreviewVideo  string         `json:"preview_video"`
	MagnetsCount  int            `json:"magnets_count"`
	HasSubtitle   bool           `json:"has_subtitle"`
	HasPreview    bool           `json:"has_preview"`
	Actors        []Actor        `json:"actors"`
	Tags          []Tag          `json:"tags"`
	Series        *Series        `json:"series,omitempty"`
	Maker         *Maker         `json:"maker,omitempty"`
	Director      *Director      `json:"director,omitempty"`
}

// APIError is returned when JavDB responds with success: 0.
type APIError struct {
	Action  string
	Message string
}

func (e *APIError) Error() string {
	if e.Action == "" && e.Message == "" {
		return "JavDB API error"
	}
	if e.Action == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Action
	}
	return fmt.Sprintf("%s: %s", e.Action, e.Message)
}

// HTTPError is returned for a non-success HTTP status.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("JavDB returned HTTP %d", e.StatusCode)
}
