package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/javdb"
)

const (
	javdbDeviceSetting = "javdb.device_uuid"
	javdbRouteSetting  = "javdb.route"
)

type MovieState string
type ReleaseStatus string

const (
	MovieNotInLibrary MovieState = "not_in_library"
	MovieSaving       MovieState = "saving"
	MovieInLibrary    MovieState = "in_library"
)

const (
	ReleaseUnknown  ReleaseStatus = "unknown"
	ReleaseReleased ReleaseStatus = "released"
	ReleaseUpcoming ReleaseStatus = "upcoming"
)

type DiscoverMovie struct {
	javdb.Movie
	State         MovieState    `json:"state"`
	ReleaseStatus ReleaseStatus `json:"release_status"`
}

type DiscoverMovieDetail struct {
	DiscoverMovie
	Zone          javdb.Zone             `json:"zone"`
	ActorMovies   []javdb.MovieReference `json:"actor_movies"`
	RelatedMovies []javdb.MovieReference `json:"related_movies"`
}

type DiscoverMagnet struct {
	javdb.Magnet
	URI string `json:"uri"`
}

type JavDBRouteStatus struct {
	Host       string                `json:"host"`
	LatencyMS  int64                 `json:"latency_ms"`
	Active     bool                  `json:"active"`
	Manual     bool                  `json:"manual"`
	Candidates []JavDBRouteCandidate `json:"candidates"`
}

type JavDBRouteCandidate struct {
	Host      string                  `json:"host"`
	LatencyMS int64                   `json:"latency_ms"`
	Status    javdb.RouteAvailability `json:"status"`
}

type persistedRoute struct {
	Host      string `json:"host"`
	LatencyMS int64  `json:"latency_ms"`
	Manual    bool   `json:"manual"`
}

// DiscoverService combines JavDB catalogue data with Miyabi's local state.
type DiscoverService struct {
	database *ent.Client
	javdb    *javdb.Client
	lists    *responseCache[[]javdb.Movie]
	details  *responseCache[javdb.MovieDetail]
	tags     *responseCache[[]javdb.TagCategory]
	magnets  *responseCache[[]javdb.Magnet]

	routeMu sync.RWMutex
	route   JavDBRouteStatus
}

// NewDiscoverService creates the lazy JavDB client and persists a stable
// anonymous device UUID. It does not perform a network request.
func NewDiscoverService(
	ctx context.Context,
	database *ent.Client,
	options javdb.Options,
) (*DiscoverService, error) {
	deviceUUID, found, err := loadSetting[string](ctx, database, javdbDeviceSetting)
	if err != nil {
		return nil, err
	}
	if !found {
		deviceUUID = options.DeviceUUID
		if deviceUUID == "" {
			deviceUUID, err = javdb.NewDeviceUUID()
			if err != nil {
				return nil, err
			}
		}
		if err := saveSetting(ctx, database, javdbDeviceSetting, deviceUUID); err != nil {
			return nil, err
		}
	}

	route, found, err := loadSetting[persistedRoute](ctx, database, javdbRouteSetting)
	if err != nil {
		return nil, err
	}
	if found {
		options.CachedHost = route.Host
		options.CachedLatency = time.Duration(route.LatencyMS) * time.Millisecond
		options.ManualRoute = route.Manual
	}
	options.DeviceUUID = deviceUUID
	client, err := javdb.New(options)
	if err != nil {
		return nil, err
	}

	return &DiscoverService{
		database: database,
		javdb:    client,
		lists:    newResponseCache[[]javdb.Movie](128, time.Minute),
		details:  newResponseCache[javdb.MovieDetail](256, 5*time.Minute),
		tags:     newResponseCache[[]javdb.TagCategory](4, 24*time.Hour),
		magnets:  newResponseCache[[]javdb.Magnet](64, time.Minute),
		route: JavDBRouteStatus{
			Host:      route.Host,
			LatencyMS: route.LatencyMS,
			Active:    route.Host != "",
			Manual:    route.Manual,
		},
	}, nil
}

func (service *DiscoverService) Close() {
	service.javdb.Close()
}

func (service *DiscoverService) Search(
	ctx context.Context,
	keyword string,
	options javdb.SearchOptions,
) ([]DiscoverMovie, error) {
	keyword = strings.TrimSpace(keyword)
	key := fmt.Sprintf("search:%q:%#v", keyword, options)
	movies, err := cachedJavDB(ctx, service, service.lists, key, func(ctx context.Context) ([]javdb.Movie, error) {
		return service.javdb.Search(ctx, keyword, options)
	})
	if err != nil {
		return nil, fmt.Errorf("search JavDB: %w", err)
	}
	return service.projectMovies(ctx, movies)
}

func (service *DiscoverService) Browse(
	ctx context.Context,
	options javdb.BrowseOptions,
) ([]DiscoverMovie, error) {
	key := fmt.Sprintf("browse:%#v", options)
	movies, err := cachedJavDB(ctx, service, service.lists, key, func(ctx context.Context) ([]javdb.Movie, error) {
		return service.javdb.Browse(ctx, options)
	})
	if err != nil {
		return nil, fmt.Errorf("browse JavDB: %w", err)
	}
	return service.projectMovies(ctx, movies)
}

func (service *DiscoverService) MovieDetail(ctx context.Context, movieID string) (DiscoverMovieDetail, error) {
	movie, err := cachedJavDB(ctx, service, service.details, movieID, func(ctx context.Context) (javdb.MovieDetail, error) {
		return service.javdb.MovieDetail(ctx, movieID)
	})
	if err != nil {
		return DiscoverMovieDetail{}, fmt.Errorf("get JavDB movie detail: %w", err)
	}
	projected, err := service.projectMovies(ctx, []javdb.Movie{movie.Movie})
	if err != nil {
		return DiscoverMovieDetail{}, err
	}
	return DiscoverMovieDetail{
		DiscoverMovie: projected[0], Zone: movie.Zone,
		ActorMovies: movie.ActorMovies, RelatedMovies: movie.RelatedMovies,
	}, nil
}

func (service *DiscoverService) Magnets(ctx context.Context, movieID string) ([]DiscoverMagnet, error) {
	magnets, err := cachedJavDB(ctx, service, service.magnets, movieID, func(ctx context.Context) ([]javdb.Magnet, error) {
		return service.javdb.Magnets(ctx, movieID)
	})
	if err != nil {
		return nil, fmt.Errorf("get JavDB magnets: %w", err)
	}
	return projectMagnets(magnets), nil
}

func projectMagnets(source []javdb.Magnet) []DiscoverMagnet {
	result := make([]DiscoverMagnet, len(source))
	for index, item := range source {
		result[index] = DiscoverMagnet{Magnet: item, URI: "magnet:?xt=urn:btih:" + item.Hash}
	}
	return result
}

func (service *DiscoverService) Media(ctx context.Context, rawURL string) (javdb.Media, error) {
	media, err := service.javdb.FetchMedia(ctx, rawURL)
	if err != nil {
		return javdb.Media{}, fmt.Errorf("fetch JavDB media: %w", err)
	}
	return media, nil
}

func (service *DiscoverService) Tags(ctx context.Context, zone javdb.Zone) ([]javdb.TagCategory, error) {
	categories, err := cachedJavDB(ctx, service, service.tags, string(zone), func(ctx context.Context) ([]javdb.TagCategory, error) {
		return service.javdb.Tags(ctx, zone)
	})
	if err != nil {
		return nil, fmt.Errorf("get JavDB tags: %w", err)
	}
	return categories, nil
}

func (service *DiscoverService) ResolveMovieID(ctx context.Context, code string) (string, error) {
	id, err := service.javdb.ResolveMovieID(ctx, code)
	if err != nil {
		return "", fmt.Errorf("resolve JavDB movie ID: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (service *DiscoverService) Route() JavDBRouteStatus {
	status, active := service.javdb.Route()
	result := JavDBRouteStatus{
		Host: status.Host, LatencyMS: status.Latency.Milliseconds(),
		Active: active, Manual: status.Manual,
		Candidates: make([]JavDBRouteCandidate, len(status.Candidates)),
	}
	for index, candidate := range status.Candidates {
		result.Candidates[index] = JavDBRouteCandidate{
			Host: candidate.Host, LatencyMS: candidate.Latency.Milliseconds(), Status: candidate.Status,
		}
	}
	if !active {
		service.routeMu.RLock()
		result.Host = service.route.Host
		result.LatencyMS = service.route.LatencyMS
		result.Manual = service.route.Manual
		service.routeMu.RUnlock()
	}
	return result
}

func (service *DiscoverService) SelectRoute(ctx context.Context, host string) (JavDBRouteStatus, error) {
	if host == "" {
		return service.Reselect(ctx)
	}
	if _, err := service.javdb.SelectRoute(ctx, host); err != nil {
		return JavDBRouteStatus{}, fmt.Errorf("select JavDB route: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return JavDBRouteStatus{}, err
	}
	return service.Route(), nil
}

func (service *DiscoverService) Reselect(ctx context.Context) (JavDBRouteStatus, error) {
	if _, err := service.javdb.Reselect(ctx); err != nil {
		return JavDBRouteStatus{}, fmt.Errorf("reselect JavDB route: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return JavDBRouteStatus{}, err
	}
	return service.Route(), nil
}

func (service *DiscoverService) projectMovies(
	ctx context.Context,
	source []javdb.Movie,
) ([]DiscoverMovie, error) {
	if len(source) == 0 {
		return []DiscoverMovie{}, nil
	}
	codes := make([]string, len(source))
	taskCodes := make([]any, len(source))
	for index, item := range source {
		codes[index] = item.Code
		taskCodes[index] = item.Code
	}

	inLibrary := make(map[string]bool, len(codes))
	libraryCodes, err := service.database.Movie.Query().Where(movie.CodeIn(codes...)).Select(movie.FieldCode).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("query local movies: %w", err)
	}
	for _, code := range libraryCodes {
		inLibrary[code] = true
	}

	saving := make(map[string]bool)
	tasks, err := service.database.Task.Query().Where(
		task.TypeEQ("offline"),
		task.StatusIn(task.StatusQueued, task.StatusRunning),
		func(selector *sql.Selector) {
			selector.Where(sqljson.ValueIn(task.FieldPayload, taskCodes, sqljson.Path("code")))
		},
	).Select(task.FieldPayload).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query active tasks: %w", err)
	}
	for _, item := range tasks {
		// The query only returns tasks whose canonical string code is on this page.
		saving[item.Payload["code"].(string)] = true
	}

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	result := make([]DiscoverMovie, len(source))
	for index, item := range source {
		state := MovieNotInLibrary
		switch {
		case inLibrary[item.Code]:
			state = MovieInLibrary
		case saving[item.Code]:
			state = MovieSaving
		}

		releaseStatus := ReleaseUnknown
		if item.ReleaseDate != "" {
			releaseDate, err := time.ParseInLocation("2006-01-02", item.ReleaseDate, time.Local)
			if err != nil {
				return nil, fmt.Errorf("parse JavDB release date %q: %w", item.ReleaseDate, err)
			}
			if releaseDate.After(today) {
				releaseStatus = ReleaseUpcoming
			} else {
				releaseStatus = ReleaseReleased
			}
		}
		result[index] = DiscoverMovie{
			Movie:         item,
			State:         state,
			ReleaseStatus: releaseStatus,
		}
	}
	return result, nil
}

func (service *DiscoverService) persistActiveRoute(ctx context.Context) error {
	service.routeMu.Lock()
	defer service.routeMu.Unlock()
	active, ok := service.javdb.Route()
	if !ok {
		return nil
	}
	route := persistedRoute{Host: active.Host, LatencyMS: active.Latency.Milliseconds(), Manual: active.Manual}
	unchanged := service.route.Active && service.route.Host == route.Host &&
		service.route.LatencyMS == route.LatencyMS && service.route.Manual == route.Manual
	if unchanged {
		return nil
	}
	if err := saveSetting(ctx, service.database, javdbRouteSetting, route); err != nil {
		return fmt.Errorf("cache JavDB route: %w", err)
	}
	service.route = JavDBRouteStatus{
		Host:      route.Host,
		LatencyMS: route.LatencyMS,
		Active:    true,
		Manual:    route.Manual,
	}
	return nil
}
