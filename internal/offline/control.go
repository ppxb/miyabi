package offline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/domain/download"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/pan"
)

func (s *Service) Cancel(ctx context.Context, id int) (domain.OfflineSubmission, error) {
	return s.advance(ctx, id, actionCancel)
}

func (s *Service) TryNext(ctx context.Context, id int) (domain.OfflineSubmission, error) {
	return s.advance(ctx, id, "next")
}

// advance serializes controls and observations for one stable task ID. Remote
// calls run outside database transactions and retain a bounded shutdown lease.
func (s *Service) advance(ctx context.Context, id int, requested string) (domain.OfflineSubmission, error) {
	unlock, err := s.lockTask(ctx, id)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	defer unlock()
	record, err := s.database.OfflineDownload.Get(ctx, id)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	source := s.drive.Source()
	if source == nil || source.AccountID != record.AccountID || source.Directory.ID != record.DirectoryID {
		return domain.OfflineSubmission{}, drive.ErrSourceChanged
	}
	if record.Status == offlinedownload.StatusDone || record.Status == offlinedownload.StatusCancelled {
		return s.submission(ctx, record, source)
	}
	cfg, prefs, err := s.config(ctx)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	state := recovery(record, prefs)
	if requested == actionSwitch && !cfg.AutoSwitch {
		return s.submission(ctx, record, source)
	}
	if requested == actionCancel {
		state.Action, state.NextHash = actionCancel, ""
	} else if (requested == actionSwitch || requested == "next") && state.Action == "" {
		next, err := s.nextCandidate(ctx, record, state)
		if err != nil {
			return domain.OfflineSubmission{}, s.deferAction(ctx, record, state, err)
		}
		if next == "" {
			state.Exhausted, state.Stalled = true, record.Status == offlinedownload.StatusRunning
			if err := s.database.OfflineDownload.UpdateOneID(id).SetRecovery(state).Exec(ctx); err != nil {
				return domain.OfflineSubmission{}, err
			}
			s.tasks.NotifyOfflineChanged()
			record.Recovery = state
			return s.submission(ctx, record, source)
		}
		state.Action, state.NextHash = actionSwitch, next
		state.Manual = requested == "next"
		state.Reason = "下载长时间无进展"
		if record.Status == offlinedownload.StatusFailed {
			state.Reason = "115 离线任务失败"
		}
	}
	if state.Action == "" {
		return s.submission(ctx, record, source)
	}
	sess, err := s.drive.OpenSource(ctx, *source)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	key := currentHash(record)
	if state.Action == actionSwitch {
		key = state.NextHash
	}
	unlockHash, err := s.operations.Lock(ctx, record.AccountID, key)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	defer unlockHash()
	current, err := s.database.OfflineDownload.Get(ctx, id)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	if current.Status == offlinedownload.StatusDone || current.Status == offlinedownload.StatusCancelled {
		return s.submission(ctx, current, source)
	}
	if requested == actionCancel {
		state = recovery(current, prefs)
		state.Action, state.NextHash = actionCancel, ""
	} else if record.Recovery != nil && record.Recovery.Action == actionSubmit && current.Recovery != nil && current.Recovery.Action == "" {
		return s.submission(ctx, current, source)
	}
	record = current
	if err := ctx.Err(); err != nil {
		return domain.OfflineSubmission{}, err
	}
	done, ok := s.drive.StartWork()
	if !ok {
		return domain.OfflineSubmission{}, context.Canceled
	}
	defer done()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.submitTimeout)
	defer cancel()
	if err := sess.Commit(ctx, func(tx *ent.Tx) error {
		return tx.OfflineDownload.UpdateOneID(id).SetRecovery(state).SetStatus(offlinedownload.StatusRunning).Exec(ctx)
	}); err != nil {
		return domain.OfflineSubmission{}, err
	}
	s.tasks.NotifyOfflineChanged()
	if state.Action == actionSubmit {
		err = s.resumeSubmission(ctx, sess, record, state)
	} else {
		err = s.removeCurrent(ctx, sess, record, state)
	}
	if err != nil {
		return domain.OfflineSubmission{}, s.deferAction(ctx, record, state, err)
	}
	record, err = s.database.OfflineDownload.Get(ctx, id)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	s.tasks.NotifyOfflineChanged()
	return s.submission(ctx, record, source)
}

func (s *Service) removeCurrent(ctx context.Context, sess drive.Session, record *ent.OfflineDownload, state *download.Recovery) error {
	hash := record.InfoHash
	if hash == "" {
		hash = currentHash(record)
	}
	remote, found, err := s.lookupRemoteTask(ctx, sess, hash)
	if err != nil {
		return err
	}
	if found && remote.Status == 2 {
		return sess.Commit(ctx, func(tx *ent.Tx) error { return s.completeTask(ctx, tx, record, remote.FileID) })
	}
	if found && remote.Status != 0 && remote.Status != 1 && remote.Status != -1 {
		return fmt.Errorf("115 returned unknown offline status %d", remote.Status)
	}
	if state.Action == actionSwitch && !state.RemovalStarted {
		if !found {
			return domain.E(domain.KindConflict, "115 中未找到原任务，请确认下载结果后重试", nil)
		}
		// A fresh observation always wins over an earlier stall decision.
		if !state.Manual && remote.Status != -1 && (remote.ProgressUnknown || rawProgress(remote) > state.Progress || remote.Status == 0) {
			state.Action, state.NextHash, state.Reason = "", "", ""
			state.Stalled = false
			state.ProgressAt, state.ObservedAt, state.Progress = s.now(), s.now(), rawProgress(remote)
			return sess.Commit(ctx, func(tx *ent.Tx) error {
				return tx.OfflineDownload.UpdateOneID(record.ID).SetRecovery(state).SetProgress(remote.Progress).ClearError().Exec(ctx)
			})
		}
		present, err := s.database.Movie.Query().Where(movie.JavdbIDEQ(record.JavdbID), movie.HasFilesWith(
			file.AccountIDEQ(record.AccountID), file.RootIDEQ(record.DirectoryID))).Exist(ctx)
		if err != nil {
			return err
		}
		if present {
			state.Action, state.NextHash, state.Reason, state.Exhausted = "", "", "影片已入库，停止换源", true
			return sess.Commit(ctx, func(tx *ent.Tx) error { return tx.OfflineDownload.UpdateOneID(record.ID).SetRecovery(state).Exec(ctx) })
		}
		occupied, err := s.database.OfflineDownload.Query().Where(offlinedownload.AccountIDEQ(record.AccountID),
			candidateHash(state.NextHash), offlinedownload.StatusEQ(offlinedownload.StatusRunning), offlinedownload.IDNEQ(record.ID)).Exist(ctx)
		if err != nil {
			return err
		}
		if occupied {
			return domain.E(domain.KindConflict, "候选磁力已有下载任务，稍后重试", nil)
		}
	}
	if found {
		state.RemovalStarted = true
		if err := sess.Commit(ctx, func(tx *ent.Tx) error { return tx.OfflineDownload.UpdateOneID(record.ID).SetRecovery(state).Exec(ctx) }); err != nil {
			return err
		}
		if err := sess.RemoveOffline(ctx, hash); err != nil {
			return err
		}
	}
	return sess.Commit(ctx, func(tx *ent.Tx) error {
		update := tx.OfflineDownload.UpdateOneID(record.ID).ClearError()
		if state.Action == actionCancel {
			state.Action, state.NextHash, state.Reason = "", "", "已取消下载"
			state.RetryAt = time.Time{}
			return update.SetStatus(offlinedownload.StatusCancelled).SetRecovery(state).Exec(ctx)
		}
		next := state.NextHash
		state.CurrentHash = next
		state.Attempts[len(state.Attempts)-1].Reason = state.Reason
		state.Attempts = append(state.Attempts, download.Attempt{Hash: next, StartedAt: s.now()})
		state.Action, state.NextHash = actionSubmit, ""
		state.RemovalStarted, state.SubmissionStarted, state.Stalled, state.Exhausted = false, false, false, false
		state.ObservedAt, state.ProgressAt, state.RetryAt = time.Time{}, time.Time{}, time.Time{}
		state.Progress, state.Failures = 0, 0
		return update.SetStatus(offlinedownload.StatusRunning).SetInfoHash(next).SetProgress(0).
			SetFileID("").SetFileIds(nil).SetAwaitingLocation(false).SetRecovery(state).Exec(ctx)
	})
}

func (s *Service) resumeSubmission(ctx context.Context, sess drive.Session, record *ent.OfflineDownload, state *download.Recovery) error {
	hash := currentHash(record)
	other, err := s.database.OfflineDownload.Query().Where(offlinedownload.AccountIDEQ(record.AccountID),
		candidateHash(hash), offlinedownload.StatusEQ(offlinedownload.StatusRunning), offlinedownload.IDNEQ(record.ID)).Exist(ctx)
	if err != nil {
		return err
	}
	if other {
		return domain.E(domain.KindConflict, "该磁力已有下载任务", nil)
	}
	if state.SubmissionStarted {
		remote, found, err := s.lookupRemoteTask(ctx, sess, hash)
		if err != nil {
			return err
		}
		if found && (remote.Status == 0 || remote.Status == 1 || remote.Status == 2) {
			if remote.DirectoryID != record.DirectoryID {
				return domain.E(domain.KindConflict, "115 任务位于其他目录", nil)
			}
			return s.recordSubmission(ctx, record, state, remote)
		}
	}
	state.SubmissionStarted = true
	if err := sess.Commit(ctx, func(tx *ent.Tx) error { return tx.OfflineDownload.UpdateOneID(record.ID).SetRecovery(state).Exec(ctx) }); err != nil {
		return err
	}
	remote, err := s.submit(ctx, sess, hash)
	if err != nil {
		return err
	}
	return s.recordSubmission(ctx, record, state, remote)
}

func (s *Service) recordSubmission(ctx context.Context, record *ent.OfflineDownload, state *download.Recovery, remote pan.OfflineTask) error {
	state.Action, state.NextHash = "", ""
	state.SubmissionStarted, state.RemovalStarted = false, false
	state.RetryAt, state.Failures = time.Time{}, 0
	state.Attempts[len(state.Attempts)-1].InfoHash = strings.ToLower(remote.Hash)
	state.ObservedAt, state.ProgressAt, state.Progress, state.RemoteStatus = s.now(), s.now(), rawProgress(remote), remote.Status
	return s.drive.Commit(ctx, func(tx *ent.Tx) error {
		if err := tx.OfflineDownload.UpdateOneID(record.ID).SetInfoHash(remote.Hash).SetRecovery(state).SetProgress(remote.Progress).ClearError().Exec(ctx); err != nil {
			return err
		}
		if remote.Status == 2 {
			record.Recovery = state
			return s.completeTask(ctx, tx, record, remote.FileID)
		}
		return nil
	})
}

func rawProgress(remote pan.OfflineTask) float64 {
	return max(remote.RawProgress, float64(remote.Progress))
}
