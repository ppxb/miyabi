package service

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/setting"
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

type JavDBRouteStatus struct {
	Host      string `json:"host"`
	LatencyMS int64  `json:"latency_ms"`
	Active    bool   `json:"active"`
}

type persistedRoute struct {
	Host      string `json:"host"`
	LatencyMS int64  `json:"latency_ms"`
}

// DiscoverService combines JavDB catalogue data with Miyabi's local state.
type DiscoverService struct {
	database *ent.Client
	javdb    *javdb.Client

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
	}
	options.DeviceUUID = deviceUUID
	client, err := javdb.New(options)
	if err != nil {
		return nil, err
	}

	return &DiscoverService{
		database: database,
		javdb:    client,
		route: JavDBRouteStatus{
			Host:      route.Host,
			LatencyMS: route.LatencyMS,
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
	movies, err := service.javdb.Search(ctx, keyword, options)
	if err != nil {
		return nil, fmt.Errorf("search JavDB: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return nil, err
	}
	return service.projectMovies(ctx, movies)
}

func (service *DiscoverService) Browse(
	ctx context.Context,
	options javdb.BrowseOptions,
) ([]DiscoverMovie, error) {
	movies, err := service.javdb.Browse(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("browse JavDB: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return nil, err
	}
	return service.projectMovies(ctx, movies)
}

func (service *DiscoverService) MovieDetail(ctx context.Context, movieID string) (DiscoverMovie, error) {
	movie, err := service.javdb.MovieDetail(ctx, movieID)
	if err != nil {
		return DiscoverMovie{}, fmt.Errorf("get JavDB movie detail: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return DiscoverMovie{}, err
	}
	projected, err := service.projectMovies(ctx, []javdb.Movie{movie})
	if err != nil {
		return DiscoverMovie{}, err
	}
	return projected[0], nil
}

func (service *DiscoverService) Tags(ctx context.Context, zone javdb.Zone) ([]javdb.TagCategory, error) {
	categories, err := service.javdb.Tags(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("get JavDB tags: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return nil, err
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
	if active, ok := service.javdb.Route(); ok {
		return JavDBRouteStatus{
			Host:      active.Host,
			LatencyMS: active.Latency.Milliseconds(),
			Active:    true,
		}
	}
	service.routeMu.RLock()
	defer service.routeMu.RUnlock()
	return service.route
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
	codes := make([]string, len(source))
	for index, item := range source {
		codes[index] = item.Code
	}

	inLibrary := make(map[string]bool, len(codes))
	if len(codes) != 0 {
		movies, err := service.database.Movie.Query().Where(movie.CodeIn(codes...)).All(ctx)
		if err != nil {
			return nil, fmt.Errorf("query local movies: %w", err)
		}
		for _, item := range movies {
			inLibrary[item.Code] = true
		}
	}

	saving := make(map[string]bool)
	tasks, err := service.database.Task.Query().Where(
		task.StatusIn(task.StatusQueued, task.StatusRunning),
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query active tasks: %w", err)
	}
	for _, item := range tasks {
		value, ok := item.Payload["code"]
		if !ok {
			continue
		}
		code, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("task %d payload code is not a string", item.ID)
		}
		if normalized := codeid.Normalize(code); normalized != "" {
			saving[normalized] = true
		}
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
	active, ok := service.javdb.Route()
	if !ok {
		return nil
	}
	route := persistedRoute{Host: active.Host, LatencyMS: active.Latency.Milliseconds()}
	service.routeMu.RLock()
	unchanged := service.route.Active && service.route.Host == route.Host &&
		service.route.LatencyMS == route.LatencyMS
	service.routeMu.RUnlock()
	if unchanged {
		return nil
	}
	if err := saveSetting(ctx, service.database, javdbRouteSetting, route); err != nil {
		return fmt.Errorf("cache JavDB route: %w", err)
	}
	service.routeMu.Lock()
	service.route = JavDBRouteStatus{
		Host:      route.Host,
		LatencyMS: route.LatencyMS,
		Active:    true,
	}
	service.routeMu.Unlock()
	return nil
}

func loadSetting[T any](
	ctx context.Context,
	database *ent.Client,
	key string,
) (T, bool, error) {
	var value T
	record, err := database.Setting.Query().Where(setting.Key(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return value, false, nil
	}
	if err != nil {
		return value, false, fmt.Errorf("load setting %s: %w", key, err)
	}
	if err := json.Unmarshal(record.Value, &value); err != nil {
		return value, false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return value, true, nil
}

func saveSetting(ctx context.Context, database *ent.Client, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode setting %s: %w", key, err)
	}
	if err := database.Setting.Create().
		SetKey(key).
		SetValue(jsontext.Value(encoded)).
		OnConflictColumns(setting.FieldKey).
		UpdateNewValues().
		Exec(ctx); err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}
