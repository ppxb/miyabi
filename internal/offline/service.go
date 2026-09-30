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
	HasMagnet(ctx context.Context, movieID, hash string) (bool, error)
	MovieCode(ctx context.Context, movieID string) (string, error)
}

// Library schedules scans and projects their current workflow status.
type Library interface {
	Workflows(context.Context, []*ent.Task) ([]domain.TaskInfo, error)
	EnqueueTargetedScan(ctx context.Context, tx *ent.Tx, source domain.LibrarySource, targetID string, offlineTaskID int, code, javdbID string) (int, error)
}

// Service coordinates 115 offline download submissions, remote polling, and indexing transitions.
type Service struct {
	database  *ent.Client
	catalogue Catalogue
	drive     *drive.Drive
	tasks     *tasks.Service
	library   Library
	// submitTimeout bounds one remote submission once it has started; the
	// caller may have gone away but the mutation must be recorded.
	submitTimeout time.Duration
	operations    offlineOperations
}

// New creates a new offline download management service.
func New(database *ent.Client, catalogue Catalogue, drive *drive.Drive, tasks *tasks.Service, library Library, submitTimeout time.Duration) *Service {
	return &Service{
		database:  database,
		catalogue: catalogue,
		drive:     drive,
		tasks:     tasks,
		library:   library,

		submitTimeout: submitTimeout,
	}
}
