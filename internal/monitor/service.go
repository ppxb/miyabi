package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/domain/download"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/subscription"
	"github.com/ppxb/miyabi/internal/magnet"
	"github.com/ppxb/miyabi/internal/tasks"
)

const (
	subscriptionConfigSetting = "subscription.config"
	defaultCheckTime          = "00:00"
)

var checkTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Config is the subscription settings section: default auto-download flags,
// the daily time at which movie and actor subscriptions are checked, and the
// magnet preferences the picker applies.
type Config struct {
	MovieAutoDownload bool               `json:"movie_auto_download"`
	ActorAutoDownload bool               `json:"actor_auto_download"`
	CheckTime         string             `json:"check_time"`
	Preferences       magnet.Preferences `json:"preferences"`
	Download          download.Config    `json:"download"`
}

func DefaultConfig() Config {
	return Config{MovieAutoDownload: true, CheckTime: defaultCheckTime, Preferences: magnet.DefaultPreferences(), Download: download.DefaultConfig()}
}

func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	value := plain(DefaultConfig())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = Config(value)
	return nil
}

func (c Config) normalized() Config {
	if c.CheckTime == "" {
		c.CheckTime = defaultCheckTime
	}
	c.Preferences = c.Preferences.Normalized()
	return c
}

func (c Config) validate() error {
	if !checkTimePattern.MatchString(c.CheckTime) {
		return domain.E(domain.KindInvalid, "检查时间格式应为 HH:MM", nil)
	}
	return c.Preferences.Validate()
}

// Item is the API projection of one subscription.
type Item struct {
	ID            int                 `json:"id"`
	Kind          string              `json:"kind"`
	TargetID      string              `json:"target_id"`
	Code          string              `json:"code,omitempty"`
	Title         string              `json:"title"`
	Cover         string              `json:"cover"`
	ReleaseDate   string              `json:"release_date,omitempty"`
	OriginID      *int                `json:"origin_id,omitempty"`
	AutoDownload  bool                `json:"auto_download"`
	Status        subscription.Status `json:"status"`
	Hash          string              `json:"hash,omitempty"`
	TaskID        *int                `json:"task_id,omitempty"`
	NextCheckAt   *time.Time          `json:"next_check_at,omitempty"`
	LastCheckedAt *time.Time          `json:"last_checked_at,omitempty"`
	Checks        int                 `json:"checks"`
	Error         *string             `json:"error,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// Discoverer is the catalogue surface the service needs, in domain types.
type Discoverer interface {
	MovieSummary(ctx context.Context, movieID string) (domain.MovieSummary, error)
	CatalogueMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error)
	BrowseMovies(ctx context.Context, options domain.BrowseOptions) ([]domain.Movie, error)
}

type OfflineAdder interface {
	Add(ctx context.Context, movieID string, hash string) (domain.OfflineSubmission, error)
}

// Service manages movie and actor subscriptions: it polls JavDB on the daily
// schedule, picks magnets by preference, submits them to 115, and runs the
// queued batch ingestion task.
type Service struct {
	database *ent.Client
	discover Discoverer
	offline  OfflineAdder
	tasks    *tasks.Service
	wake     chan struct{}
}

func New(database *ent.Client, discover Discoverer, offline OfflineAdder, taskSvc *tasks.Service) *Service {
	return &Service{database: database, discover: discover, offline: offline, tasks: taskSvc, wake: make(chan struct{}, 1)}
}

// Pending signals when a subscription wants an immediate check.
func (service *Service) Pending() <-chan struct{} { return service.wake }

func (service *Service) signal() {
	select {
	case service.wake <- struct{}{}:
	default:
	}
}

func (service *Service) Config(ctx context.Context) (Config, error) {
	cfg, found, err := database.LoadSetting[Config](ctx, service.database, subscriptionConfigSetting)
	if err != nil {
		return DefaultConfig(), err
	}
	if !found {
		return DefaultConfig(), nil
	}
	return cfg, nil
}

// config serves background paths, which fall back to defaults when the row is unreadable.
func (service *Service) config(ctx context.Context) Config {
	cfg, err := service.Config(ctx)
	if err != nil {
		slog.WarnContext(ctx, "subscription settings unreadable; using defaults", "error", err)
	}
	return cfg
}

func (service *Service) UpdateConfig(ctx context.Context, cfg Config) (Config, error) {
	cfg = cfg.normalized()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	if err := database.SaveSetting(ctx, service.database, subscriptionConfigSetting, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// TargetItem is a lightweight projection used to check subscription status.
type TargetItem struct {
	ID       int                 `json:"id"`
	Kind     string              `json:"kind"`
	TargetID string              `json:"target_id"`
	OriginID *int                `json:"origin_id,omitempty"`
	Status   subscription.Status `json:"status"`
}

func (service *Service) Targets(ctx context.Context, kind string) ([]TargetItem, error) {
	query := service.database.Subscription.Query()
	if kind != "" {
		query = query.Where(subscription.KindEQ(subscription.Kind(kind)))
	}
	records, err := query.Select(
		subscription.FieldID,
		subscription.FieldKind,
		subscription.FieldTargetID,
		subscription.FieldOriginID,
		subscription.FieldStatus,
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscription targets: %w", err)
	}
	result := make([]TargetItem, len(records))
	for index, record := range records {
		result[index] = TargetItem{
			ID:       record.ID,
			Kind:     string(record.Kind),
			TargetID: record.TargetID,
			OriginID: record.OriginID,
			Status:   record.Status,
		}
	}
	return result, nil
}

func (service *Service) List(ctx context.Context, kind string, page, limit int) ([]Item, error) {
	query := service.database.Subscription.Query()
	if kind != "" {
		query = query.Where(subscription.KindEQ(subscription.Kind(kind)))
	}
	query = query.Order(ent.Desc(subscription.FieldID)).Offset((page - 1) * limit).Limit(limit)
	records, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return items(records), nil
}

// ActorFeed lists the movie subscriptions spawned by one actor subscription (or all actors if actorSubscriptionID <= 0).
func (service *Service) ActorFeed(ctx context.Context, actorSubscriptionID, page, limit int) ([]Item, error) {
	query := service.database.Subscription.Query().
		Where(subscription.KindEQ(subscription.KindMovie)).
		Order(ent.Desc(subscription.FieldReleaseDate), ent.Desc(subscription.FieldID))
	if actorSubscriptionID > 0 {
		query = query.Where(subscription.OriginIDEQ(actorSubscriptionID))
	} else {
		query = query.Where(subscription.OriginIDNotNil())
	}
	records, err := query.Offset((page - 1) * limit).Limit(limit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list actor feed: %w", err)
	}
	return items(records), nil
}

func items(records []*ent.Subscription) []Item {
	result := make([]Item, len(records))
	for index, record := range records {
		result[index] = subscriptionItem(record)
	}
	return result
}

func subscriptionItem(record *ent.Subscription) Item {
	return Item{
		ID: record.ID, Kind: string(record.Kind), TargetID: record.TargetID, Code: record.Code,
		Title: record.Title, Cover: record.Cover, ReleaseDate: record.ReleaseDate, OriginID: record.OriginID,
		AutoDownload: record.AutoDownload, Status: record.Status, Hash: record.Hash,
		TaskID: record.TaskID, NextCheckAt: record.NextCheckAt, LastCheckedAt: record.LastCheckedAt,
		Checks: record.Checks, Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
