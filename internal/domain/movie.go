package domain

// MovieRef uniquely identifies a movie reference across different metadata sources.
type MovieRef struct {
	Code    string `json:"code"`
	JavDBID string `json:"javdb_id"`
	Zone    Zone   `json:"zone,omitempty"`
}

// LocalMovie represents the identity and library ID of a movie indexed locally.
type LocalMovie struct {
	ID      int
	Code    string
	JavDBID *string
}

// Zone is a movie category/section.
type Zone string

const (
	// ZoneUnknown is only returned for missing or unsupported movie types.
	// It is not a valid search, browse, or tag filter.
	ZoneUnknown    Zone = "unknown"
	ZoneAll        Zone = "all"
	ZoneCensored   Zone = "censored"
	ZoneUncensored Zone = "uncensored"
	ZoneWestern    Zone = "western"
	ZoneFC2        Zone = "fc2"
	ZoneAnime      Zone = "anime"
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
	// Zone filters category/tag browsing. Empty leaves the zone unspecified;
	// entity movies always omit it.
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

// MovieDetail represents full details of a movie.
type MovieDetail struct {
	Movie
	Zone          Zone             `json:"zone"`
	ActorMovies   []MovieReference `json:"actor_movies"`
	RelatedMovies []MovieReference `json:"related_movies"`
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

// Tag is a content tag.
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

// TagCategory groups the taxonomy returned by metadata providers.
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

// Movie is the core domain movie model.
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
