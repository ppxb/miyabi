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

// Config is the download settings section: the magnet preferences and
// download options offline downloads apply, plus the daily time at which actor
// subscriptions are checked.
type Config struct {
	CheckTime   string             `json:"check_time"`
	Preferences magnet.Preferences `json:"preferences"`
	Download    download.Config    `json:"download"`
}

func DefaultConfig() Config {
	return Config{CheckTime: defaultCheckTime, Preferences: magnet.DefaultPreferences(), Download: download.DefaultConfig()}
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
	Status        subscription.Status `json:"status"`
	NextCheckAt   *time.Time          `json:"next_check_at,omitempty"`
	LastCheckedAt *time.Time          `json:"last_checked_at,omitempty"`
	Checks        int                 `json:"checks"`
	Error         *string             `json:"error,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// Discoverer browses JavDB, the fallback source of an actor's works.
type Discoverer interface {
	BrowseMovies(ctx context.Context, options domain.BrowseOptions) ([]domain.Movie, error)
}

// ActressSource lists an actress's works on FANZA, the primary source.
type ActressSource interface {
	ResolveActress(ctx context.Context, names []string) (string, error)
	ActressWorks(ctx context.Context, actressID string) ([]domain.Movie, error)
}

// ActorIdentity is a library actor as following needs it.
type ActorIdentity struct {
	// Key is the canonical actor key subscriptions are stored under.
	Key, Name, Avatar string
	// Names holds every known spelling, the canonical name first.
	Names            []string
	FanzaID, JavdbID string
}

// ActorLookup reads a library actor by key; a missing actor is KindNotFound.
type ActorLookup func(ctx context.Context, key string) (ActorIdentity, error)

// Service manages actor subscriptions: it checks each actor's works on the
// daily schedule and records new ones as movie subscriptions for display.
// Downloading stays a manual action.
type Service struct {
	database *ent.Client
	discover Discoverer
	fanza    ActressSource
	actors   ActorLookup
	tasks    *tasks.Service
}

func New(database *ent.Client, discover Discoverer, fanza ActressSource, actors ActorLookup, taskSvc *tasks.Service) *Service {
	return &Service{database: database, discover: discover, fanza: fanza, actors: actors, tasks: taskSvc}
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
		Status: record.Status, NextCheckAt: record.NextCheckAt, LastCheckedAt: record.LastCheckedAt,
		Checks: record.Checks, Error: record.Error, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}
