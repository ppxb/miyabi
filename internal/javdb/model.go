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
	ManualRoute       bool
	DeviceUUID        string
	Proxy             string
	Timeout           time.Duration
	RequestsPerSecond float64
	Burst             int
}

// RouteStatus describes the currently selected API route.
type RouteStatus struct {
	Host       string
	Latency    time.Duration
	Manual     bool
	Candidates []RouteCandidate
}

type RouteAvailability string

const (
	RouteUntested    RouteAvailability = "untested"
	RouteAvailable   RouteAvailability = "available"
	RouteUnavailable RouteAvailability = "unavailable"
)

type RouteCandidate struct {
	Host    string
	Latency time.Duration
	Status  RouteAvailability
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
	Zone       Zone
	EntityType EntityType
	EntityID   string
	Main       []string
	TagIDs     []string
	Year       string
	Month      string
	Sort       string
	Order      string
	Page       int
	Limit      int
}

type EntityType string

const (
	EntityActor    EntityType = "actor"
	EntitySeries   EntityType = "series"
	EntityMaker    EntityType = "maker"
	EntityDirector EntityType = "director"
)

// MovieReference is the compact recommendation returned with a movie detail.
type MovieReference struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Thumbnail string `json:"thumbnail"`
}

type MovieDetail struct {
	Movie
	Zone          Zone             `json:"zone"`
	ActorMovies   []MovieReference `json:"actor_movies"`
	RelatedMovies []MovieReference `json:"related_movies"`
}

// Magnet describes a resource indexed by JavDB. Size is measured in bytes.
type Magnet struct {
	Hash        string `json:"hash"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	HasSubtitle bool   `json:"has_subtitle"`
	HD          bool   `json:"hd"`
	FilesCount  int    `json:"files_count"`
	CreatedAt   string `json:"created_at"`
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

// TagOption is a content tag or a browse option such as year or resource type.
type TagOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TagCategory groups the Traditional Chinese taxonomy returned by JavDB.
type TagCategory struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Tags []TagOption `json:"tags"`
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
