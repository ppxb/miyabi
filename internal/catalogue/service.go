package catalogue

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/javbus"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/netx"
)

const (
	javdbDeviceSetting = "javdb.device_uuid"
	javdbRouteSetting  = "javdb.route"
)

type persistedRoute struct {
	Host      string `json:"host"`
	LatencyMS int64  `json:"latency_ms"`
	Manual    bool   `json:"manual"`
}

// Service combines JavDB catalogue data with Miyabi's local library and workflow state.
type Service struct {
	database   *ent.Client
	local      LocalState
	javdb      JavDBClient
	javbus     JavBusSource
	aggregator *magnet.Aggregator

	proxy   *netx.ProxyManager
	lists   *responseCache[[]domain.Movie]
	details *responseCache[domain.MovieDetail]
	tags    *responseCache[[]domain.TagCategory]
	magnets *responseCache[[]domain.Magnet]

	persistRouteMu sync.Mutex
	// Used only to avoid redundant database writes; live state belongs to JavDB.
	lastSavedRoute persistedRoute
}

// New creates the lazy JavDB client and restores/persists device UUID and route settings.
func New(
	ctx context.Context,
	db *ent.Client,
	options javdb.Options,
	proxy *netx.ProxyManager,
	local LocalState,
) (*Service, error) {
	deviceUUID, found, err := database.LoadSetting[string](ctx, db, javdbDeviceSetting)
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
		if err := database.SaveSetting(ctx, db, javdbDeviceSetting, deviceUUID); err != nil {
			return nil, err
		}
	}

	route, found, err := database.LoadSetting[persistedRoute](ctx, db, javdbRouteSetting)
	if err != nil {
		return nil, err
	}
	if found {
		options.CachedHost = route.Host
		options.CachedLatency = time.Duration(route.LatencyMS) * time.Millisecond
		options.ManualRoute = route.Manual
	}
	options.DeviceUUID = deviceUUID
	options.Proxy = proxy
	client, err := javdb.New(options)
	if err != nil {
		return nil, err
	}

	javbusClient, err := javbus.New(javbus.Options{Proxy: proxy})
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("initialize JavBus client: %w", err)
	}

	service := newService(db, client, javbusClient, local, route)
	service.proxy = proxy
	return service, nil
}

// NewWithClients assembles explicit clients using the same cache and aggregation
// pipeline as New. JavDB is required; JavBus may be nil. Close releases both clients.
func NewWithClients(
	ctx context.Context,
	db *ent.Client,
	primary JavDBClient,
	supplement JavBusSource,
	local LocalState,
) (*Service, error) {
	route, _, err := database.LoadSetting[persistedRoute](ctx, db, javdbRouteSetting)
	if err != nil {
		return nil, err
	}
	return newService(db, primary, supplement, local, route), nil
}

// Response caches absorb repeated page loads; entries are small and short-lived
// so JavDB changes (new magnets, edited metadata) surface within minutes.
const (
	listCacheSize, listCacheTTL       = 128, time.Minute
	detailCacheSize, detailCacheTTL   = 256, 5 * time.Minute
	tagsCacheSize, tagsCacheTTL       = 5, 24 * time.Hour
	magnetsCacheSize, magnetsCacheTTL = 64, time.Minute
)

func newService(database *ent.Client, primary JavDBClient, supplement JavBusSource, local LocalState, route persistedRoute) *Service {
	sources := []magnet.Source{primary}
	if supplement != nil {
		sources = append(sources, supplement)
	}
	return &Service{
		database:       database,
		javdb:          primary,
		javbus:         supplement,
		aggregator:     magnet.NewAggregator(sources, 0),
		lists:          newResponseCache[[]domain.Movie](listCacheSize, listCacheTTL),
		details:        newResponseCache[domain.MovieDetail](detailCacheSize, detailCacheTTL),
		tags:           newResponseCache[[]domain.TagCategory](tagsCacheSize, tagsCacheTTL),
		magnets:        newResponseCache[[]domain.Magnet](magnetsCacheSize, magnetsCacheTTL),
		local:          local,
		lastSavedRoute: route,
	}
}

func (service *Service) Close() {
	if service.javdb != nil {
		service.javdb.Close()
	}
	if service.javbus != nil {
		service.javbus.Close()
	}
}

func (service *Service) Search(
	ctx context.Context,
	keyword string,
	options domain.SearchOptions,
) ([]Movie, error) {
	keyword = strings.TrimSpace(keyword)
	key := fmt.Sprintf("search:%q:%#v", keyword, options)
	movies, err := cachedJavDB(ctx, service, service.lists, key, func(ctx context.Context) ([]domain.Movie, error) {
		return service.javdb.Search(ctx, keyword, options)
	})
	if err != nil {
		return nil, fmt.Errorf("search JavDB: %w", err)
	}
	return service.projectMovies(ctx, movies)
}

func (service *Service) Browse(
	ctx context.Context,
	options domain.BrowseOptions,
) ([]Movie, error) {
	key := fmt.Sprintf("browse:%#v", options)
	movies, err := cachedJavDB(ctx, service, service.lists, key, func(ctx context.Context) ([]domain.Movie, error) {
		return service.javdb.Browse(ctx, options)
	})
	if err != nil {
		return nil, fmt.Errorf("browse JavDB: %w", err)
	}
	return service.projectMovies(ctx, movies)
}

func (service *Service) CatalogueDetail(ctx context.Context, movieID string) (domain.MovieDetail, error) {
	return cachedJavDB(ctx, service, service.details, movieID, func(ctx context.Context) (domain.MovieDetail, error) {
		detail, err := service.javdb.MovieDetail(ctx, movieID)
		if err != nil {
			return domain.MovieDetail{}, err
		}
		if err := service.completeMovieTags(ctx, &detail); err != nil {
			return domain.MovieDetail{}, err
		}
		return detail, nil
	})
}

func (service *Service) MovieDetail(ctx context.Context, movieID string) (MovieDetail, error) {
	movie, err := service.CatalogueDetail(ctx, movieID)
	if err != nil {
		return MovieDetail{}, fmt.Errorf("get JavDB movie detail: %w", err)
	}
	projected, err := service.projectMovies(ctx, []domain.Movie{movie.Movie})
	if err != nil {
		return MovieDetail{}, err
	}
	return MovieDetail{
		Movie:         projected[0],
		Zone:          movie.Zone,
		ActorMovies:   movie.ActorMovies,
		RelatedMovies: movie.RelatedMovies,
	}, nil
}

func (service *Service) MovieCode(ctx context.Context, movieID string) (string, error) {
	movie, err := service.CatalogueDetail(ctx, movieID)
	if err != nil {
		return "", err
	}
	return movie.Code, nil
}

func (service *Service) MovieSummary(ctx context.Context, movieID string) (domain.MovieSummary, error) {
	detail, err := service.MovieDetail(ctx, movieID)
	if err != nil {
		return domain.MovieSummary{}, err
	}
	return domain.MovieSummary{
		ID:          detail.ID,
		Code:        detail.Code,
		Title:       detail.Title,
		Cover:       detail.Cover,
		ReleaseDate: detail.ReleaseDate,
	}, nil
}

// BrowseMovies returns browsed movies as domain types for callers requiring domain boundaries.
func (service *Service) BrowseMovies(ctx context.Context, options domain.BrowseOptions) ([]domain.Movie, error) {
	movies, err := service.Browse(ctx, options)
	if err != nil {
		return nil, err
	}
	res := make([]domain.Movie, len(movies))
	for i, m := range movies {
		res[i] = m.Movie
	}
	return res, nil
}

func (service *Service) Media(ctx context.Context, rawURL string) (domain.Media, error) {
	media, err := service.javdb.FetchMedia(ctx, rawURL)
	if err != nil {
		return domain.Media{}, fmt.Errorf("fetch JavDB media: %w", err)
	}
	return media, nil
}

func (service *Service) Tags(ctx context.Context, zone domain.Zone) ([]domain.TagCategory, error) {
	categories, err := cachedJavDB(ctx, service, service.tags, string(zone), func(ctx context.Context) ([]domain.TagCategory, error) {
		return service.javdb.Tags(ctx, zone)
	})
	if err != nil {
		return nil, fmt.Errorf("get JavDB tags: %w", err)
	}
	return categories, nil
}

func (service *Service) ResolveMovieID(ctx context.Context, code string) (string, error) {
	id, err := service.javdb.ResolveMovieID(ctx, code)
	if err != nil {
		return "", fmt.Errorf("resolve JavDB movie ID: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (service *Service) Route() RouteStatus {
	status, active := service.javdb.Route()
	result := RouteStatus{
		Host:       status.Host,
		LatencyMS:  status.Latency.Milliseconds(),
		Active:     active,
		Manual:     status.Manual,
		Candidates: make([]RouteCandidate, len(status.Candidates)),
	}
	for index, candidate := range status.Candidates {
		result.Candidates[index] = RouteCandidate{
			Host:      candidate.Host,
			LatencyMS: candidate.Latency.Milliseconds(),
			Status:    candidate.Status,
		}
	}
	return result
}

func (service *Service) SelectRoute(ctx context.Context, host string) (RouteStatus, error) {
	if host == "" {
		return service.Reselect(ctx)
	}
	if _, err := service.javdb.SelectRoute(ctx, host); err != nil {
		return RouteStatus{}, fmt.Errorf("select JavDB route: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return RouteStatus{}, err
	}
	return service.Route(), nil
}

func (service *Service) Reselect(ctx context.Context) (RouteStatus, error) {
	if _, err := service.javdb.Reselect(ctx); err != nil {
		return RouteStatus{}, fmt.Errorf("reselect JavDB route: %w", err)
	}
	if err := service.persistActiveRoute(ctx); err != nil {
		return RouteStatus{}, err
	}
	return service.Route(), nil
}

// Facets exposes available zones and sort options for front-end discovery filtering.
func (service *Service) Facets() Facets {
	return Facets{
		Zones: []FacetItem{
			{Value: string(domain.ZoneCensored), Label: "有码"},
			{Value: string(domain.ZoneUncensored), Label: "无码"},
			{Value: string(domain.ZoneFC2), Label: "FC2"},
			{Value: string(domain.ZoneWestern), Label: "欧美"},
			{Value: string(domain.ZoneAnime), Label: "动漫"},
		},
		Sorts: []FacetItem{
			{Value: "release", Label: "发布日期"},
			{Value: "update", Label: "更新日期"},
			{Value: "hit", Label: "热度"},
			{Value: "score", Label: "评分"},
		},
	}
}

func (service *Service) projectMovies(
	ctx context.Context,
	source []domain.Movie,
) ([]Movie, error) {
	if len(source) == 0 {
		return []Movie{}, nil
	}
	identities := make([]MovieIdentity, len(source))
	for index, item := range source {
		identities[index] = MovieIdentity{ID: item.ID, Code: item.Code}
	}
	states, err := service.MovieStates(ctx, identities)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	result := make([]Movie, len(source))
	for index, item := range source {
		releaseStatus := ReleaseUnknown
		item.ReleaseDate = strings.TrimSpace(item.ReleaseDate)
		if item.ReleaseDate != "" {
			releaseDate, err := time.ParseInLocation("2006-01-02", item.ReleaseDate, time.Local)
			if err != nil {
				slog.WarnContext(ctx, "invalid JavDB release date; omitting date",
					"movie_id", item.ID, "field", "release_date", "value", item.ReleaseDate)
				item.ReleaseDate = ""
			} else if releaseDate.After(today) {
				releaseStatus = ReleaseUpcoming
			} else {
				releaseStatus = ReleaseReleased
			}
		}
		result[index] = Movie{
			Movie:         item,
			LibraryID:     states[index].LibraryID,
			State:         states[index].State,
			ReleaseStatus: releaseStatus,
		}
	}
	return result, nil
}

func (service *Service) persistActiveRoute(ctx context.Context) error {
	service.persistRouteMu.Lock()
	defer service.persistRouteMu.Unlock()
	active, ok := service.javdb.Route()
	if !ok {
		return nil
	}
	route := persistedRoute{Host: active.Host, LatencyMS: active.Latency.Milliseconds(), Manual: active.Manual}
	if route == service.lastSavedRoute {
		return nil
	}
	if err := database.SaveSetting(ctx, service.database, javdbRouteSetting, route); err != nil {
		return fmt.Errorf("cache JavDB route: %w", err)
	}
	service.lastSavedRoute = route
	return nil
}
