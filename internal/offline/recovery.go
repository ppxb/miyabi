package offline

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/domain/download"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	"github.com/ppxb/miyabi/internal/magnet"
)

const (
	actionCancel   = "cancel"
	actionSwitch   = "switch"
	actionSubmit   = "submit"
	observationGap = 90 * time.Second
)

// Recovery thresholds are internal policy, independent of user preferences.
type recoveryPolicy struct {
	zeroProgressTimeout time.Duration
	stalledTimeout      time.Duration
	completionGrace     time.Duration
	maxAttempts         int
}

func (p recoveryPolicy) timeout(progress float64) time.Duration {
	if progress == 0 {
		return p.zeroProgressTimeout
	}
	if progress >= 95 {
		return max(p.stalledTimeout, p.completionGrace)
	}
	return p.stalledTimeout
}

func recovery(record *ent.OfflineDownload, prefs magnet.Preferences) *download.Recovery {
	if record.Recovery != nil {
		state := *record.Recovery
		state.Attempts = slices.Clone(state.Attempts)
		return &state
	}
	return &download.Recovery{
		Preferences: prefs,
		CurrentHash: record.Hash,
		Attempts:    []download.Attempt{{Hash: record.Hash, InfoHash: record.InfoHash, StartedAt: record.CreatedAt}},
	}
}

// Hash remains the immutable history key; CurrentHash identifies the active
// candidate. Switching to a previously used hash must not hide this task.
func currentHash(record *ent.OfflineDownload) string {
	if record.Recovery != nil && record.Recovery.CurrentHash != "" {
		return record.Recovery.CurrentHash
	}
	return record.Hash
}

func candidateHash(hash string) predicate.OfflineDownload {
	return offlinedownload.Or(offlinedownload.HashEQ(hash), offlinedownload.InfoHashEQ(hash), func(s *sql.Selector) {
		s.Where(sqljson.ValueEQ(offlinedownload.FieldRecovery, hash, sqljson.Path("current_hash")))
	})
}

func (s *Service) lockTask(ctx context.Context, id int) (func(), error) {
	return s.operations.Lock(ctx, "task", fmt.Sprint(id))
}

// Recover advances at most one remote action per poll; all tasks are observed
// before a slow submission can occupy the serial poller.
func (s *Service) Recover(ctx context.Context) error {
	cfg, prefs, err := s.config(ctx)
	if err != nil {
		return err
	}
	source := s.drive.Source()
	if source == nil {
		return nil
	}
	records, err := s.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.DirectoryIDEQ(source.Directory.ID),
		offlinedownload.StatusIn(offlinedownload.StatusRunning, offlinedownload.StatusFailed)).
		Order(ent.Asc(offlinedownload.FieldID)).All(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		state := recovery(record, prefs)
		if s.now().Before(state.RetryAt) {
			continue
		}
		if state.Action != "" {
			// Turning automation off never abandons a submission whose outcome
			// is unknown, or a cancellation explicitly requested by the user.
			if state.Action == actionSwitch && !cfg.AutoSwitch && !state.Manual {
				continue
			}
			_, err := s.advance(ctx, record.ID, "")
			return err
		}
		if state.Exhausted {
			continue
		}
		failed := record.Status == offlinedownload.StatusFailed && state.RemoteStatus == -1
		stalled := record.Status == offlinedownload.StatusRunning && state.RemoteStatus == 1 &&
			!state.ProgressAt.IsZero() && s.now().Sub(state.ObservedAt) <= observationGap &&
			s.now().Sub(state.ProgressAt) >= s.policy.timeout(state.Progress)
		if !failed && !stalled {
			continue
		}
		if !cfg.AutoSwitch {
			if stalled && !state.Stalled {
				if err := s.markStalled(ctx, record.ID); err != nil {
					return err
				}
			}
			continue
		}
		_, err := s.advance(ctx, record.ID, actionSwitch)
		return err
	}
	return nil
}

func (s *Service) markStalled(ctx context.Context, id int) error {
	unlock, err := s.lockTask(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()
	record, err := s.database.OfflineDownload.Get(ctx, id)
	if err != nil {
		return err
	}
	if record.Status != offlinedownload.StatusRunning || record.Recovery == nil {
		return nil
	}
	state := recovery(record, record.Recovery.Preferences)
	if state.Action != "" || state.Stalled || state.RemoteStatus != 1 || s.now().Sub(state.ObservedAt) > observationGap || s.now().Sub(state.ProgressAt) < s.policy.timeout(state.Progress) {
		return nil
	}
	state.Stalled = true
	if err := s.database.OfflineDownload.UpdateOneID(id).SetRecovery(state).Exec(ctx); err != nil {
		return err
	}
	s.tasks.NotifyOfflineChanged()
	return nil
}

func (s *Service) nextCandidate(ctx context.Context, record *ent.OfflineDownload, state *download.Recovery) (string, error) {
	if len(state.Attempts) >= s.policy.maxAttempts {
		state.Reason = "已达磁力尝试上限"
		return "", nil
	}
	magnets, err := s.catalogue.CatalogueMagnets(ctx, record.JavdbID)
	if err != nil {
		return "", err
	}
	tried := make(map[string]bool, len(state.Attempts)*2)
	for _, attempt := range state.Attempts {
		tried[strings.ToLower(attempt.Hash)] = true
		tried[strings.ToLower(attempt.InfoHash)] = true
	}
	// Do not attach a replacement to another user-initiated download.
	active, err := s.database.OfflineDownload.Query().Where(offlinedownload.AccountIDEQ(record.AccountID),
		offlinedownload.StatusEQ(offlinedownload.StatusRunning), offlinedownload.IDNEQ(record.ID)).All(ctx)
	if err != nil {
		return "", err
	}
	for _, other := range active {
		tried[strings.ToLower(other.Hash)] = true
		tried[strings.ToLower(currentHash(other))] = true
		tried[strings.ToLower(other.InfoHash)] = true
	}
	candidates := make([]domain.Magnet, 0, len(magnets))
	for _, candidate := range magnets {
		if !tried[strings.ToLower(candidate.Hash)] {
			candidates = append(candidates, candidate)
		}
	}
	best, found := magnet.NewPicker(state.Preferences).Pick(candidates)
	if !found {
		state.Reason = "暂无其他符合偏好的磁力"
		return "", nil
	}
	return strings.ToLower(best.Hash), nil
}

func (s *Service) deferAction(ctx context.Context, record *ent.OfflineDownload, state *download.Recovery, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	current, err := s.database.OfflineDownload.Get(ctx, record.ID)
	if err != nil {
		return err
	}
	if current.Recovery != nil {
		state = recovery(current, state.Preferences)
	}
	state.Failures++
	wait := time.Minute * time.Duration(1<<min(state.Failures-1, 6))
	if delay, ok := domain.RetryDelay(cause); ok {
		wait = max(wait, delay)
	}
	state.RetryAt = s.now().Add(wait)
	if err := s.database.OfflineDownload.UpdateOneID(record.ID).SetRecovery(state).SetError(domain.PublicMessage(cause)).Exec(ctx); err != nil {
		return err
	}
	s.tasks.NotifyOfflineChanged()
	return cause
}
