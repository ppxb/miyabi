package offline

import (
	"context"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/tasks"
)

// ErrMagnetNotFound is returned when attempting to add a magnet hash that does not belong to the movie.
var ErrMagnetNotFound = domain.E(domain.KindInvalid, "磁力链不属于当前影片，请刷新后重试", nil)

// Catalogue supplies movie metadata and magnet availability verification.
type Catalogue interface {
	CatalogueMagnets(ctx context.Context, movieID string) ([]domain.Magnet, error)
	CatalogueMagnetsByCode(ctx context.Context, code string) ([]domain.Magnet, error)
}

// MetadataWarmer pre-fetches metadata for a code so a later library scrape can
// reuse the cached sources instead of hitting the networks again.
type MetadataWarmer interface {
	Resolve(context.Context, domain.MovieRef) (domain.MovieMetadata, error)
}

// Library schedules scans and projects their current workflow status.
type Library interface {
	Workflows(context.Context, []*ent.Task) ([]domain.TaskInfo, error)
	EnqueueTargetedScan(ctx context.Context, tx *ent.Tx, source domain.LibrarySource, targetID string, offlineTaskID int, code string) (int, error)
}

// Service coordinates 115 offline download submissions, remote polling, and indexing transitions.
type Service struct {
	database  *ent.Client
	catalogue Catalogue
	drive     *drive.Drive
	tasks     *tasks.Service
	library   Library
	metadata  MetadataWarmer
	// submitTimeout bounds one remote submission once it has started; the
	// caller may have gone away but the mutation must be recorded.
	submitTimeout time.Duration
	operations    offlineOperations
	now           func() time.Time
	policy        recoveryPolicy
}

// New creates a new offline download management service. metadata prewarms the
// scrape cache when a movie is added to the drive; nil disables prewarming.
func New(database *ent.Client, catalogue Catalogue, drive *drive.Drive, tasks *tasks.Service, library Library, metadata MetadataWarmer, submitTimeout time.Duration) *Service {
	return &Service{
		database:  database,
		catalogue: catalogue,
		drive:     drive,
		tasks:     tasks,
		library:   library,
		metadata:  metadata,

		submitTimeout: submitTimeout,
		now:           time.Now,
		policy: recoveryPolicy{
			zeroProgressTimeout: 15 * time.Minute,
			stalledTimeout:      30 * time.Minute,
			completionGrace:     60 * time.Minute,
			maxAttempts:         3,
		},
	}
}
