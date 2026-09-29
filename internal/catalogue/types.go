package catalogue

import (
	"context"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/magnet"
)

type MovieState string
type ReleaseStatus string

const (
	MovieNotInLibrary MovieState = "not_in_library"
	MovieSaving       MovieState = "saving"
	MovieProcessing   MovieState = "processing"
	MovieInLibrary    MovieState = "in_library"
)

const (
	ReleaseUnknown  ReleaseStatus = "unknown"
	ReleaseReleased ReleaseStatus = "released"
	ReleaseUpcoming ReleaseStatus = "upcoming"
)

// Movie combines JavDB catalogue metadata with local library presence and release status.
type Movie struct {
	domain.Movie
	LibraryID     int           `json:"library_id,omitempty"`
	State         MovieState    `json:"state"`
	ReleaseStatus ReleaseStatus `json:"release_status"`
}

// MovieDetail represents a full movie page including recommendations and zone.
type MovieDetail struct {
	Movie
	Zone          domain.Zone             `json:"zone"`
	ActorMovies   []domain.MovieReference `json:"actor_movies"`
	RelatedMovies []domain.MovieReference `json:"related_movies"`
}

// Magnet represents a torrent magnet link with constructed URI.
type Magnet struct {
	domain.Magnet
	URI string `json:"uri"`
}

// RouteStatus exposes JavDB routing status and candidates.
type RouteStatus struct {
	Host       string           `json:"host"`
	LatencyMS  int64            `json:"latency_ms"`
	Active     bool             `json:"active"`
	Manual     bool             `json:"manual"`
	Candidates []RouteCandidate `json:"candidates"`
}

// RouteCandidate represents a probed JavDB mirror candidate.
type RouteCandidate struct {
	Host      string                  `json:"host"`
	LatencyMS int64                   `json:"latency_ms"`
	Status    javdb.RouteAvailability `json:"status"`
}

// MovieIdentity specifies a movie by its catalogue ID and release code.
type MovieIdentity struct {
	ID   string `json:"id" binding:"required,max=200"`
	Code string `json:"code" binding:"required,max=200"`
}

// MovieStateItem represents the resolution result of a movie identity against local library and tasks.
type MovieStateItem struct {
	ID        string     `json:"id"`
	LibraryID int        `json:"library_id,omitempty"`
	State     MovieState `json:"state"`
}

// FacetItem represents an option for discovery filtering.
type FacetItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Facets exposes discovery filtering dimensions such as zones and sorting options.
type Facets struct {
	Zones []FacetItem `json:"zones"`
	Sorts []FacetItem `json:"sorts"`
}

// JavDBClient supplies the primary catalogue, route management and magnets.
type JavDBClient interface {
	magnet.Source
	Close()
	Search(context.Context, string, domain.SearchOptions) ([]domain.Movie, error)
	Browse(context.Context, domain.BrowseOptions) ([]domain.Movie, error)
	MovieDetail(context.Context, string) (domain.MovieDetail, error)
	FetchMedia(context.Context, string) (domain.Media, error)
	Tags(context.Context, domain.Zone) ([]domain.TagCategory, error)
	ResolveMovieID(context.Context, string) (string, error)
	Route() (javdb.RouteStatus, bool)
	SelectRoute(context.Context, string) (javdb.RouteStatus, error)
	Reselect(context.Context) (javdb.RouteStatus, error)
}

// JavBusSource supplements magnets without providing catalogue metadata.
type JavBusSource interface {
	magnet.Source
	Available() bool
	Close()
}

// LocalState supplies the mounted library source and matches local movies.
// It is implemented by library.Service to decouple catalogue from library storage.
type LocalState interface {
	Source() *domain.LibrarySource
	MatchingMovies(ctx context.Context, javdbIDs []string, codes []string) ([]domain.LocalMovie, error)
}
