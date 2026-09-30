package offline

import (
	"context"
	"fmt"
	"strings"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
)

// Add validates and records a new offline download for the given movie ID and magnet hash.
func (service *Service) Add(ctx context.Context, movieID, hash string) (domain.OfflineSubmission, error) {
	hash = strings.ToLower(hash)
	has, err := service.catalogue.HasMagnet(ctx, movieID, hash)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	if !has {
		return domain.OfflineSubmission{}, ErrMagnetNotFound
	}
	rawCode, err := service.catalogue.MovieCode(ctx, movieID)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	code := codeid.Normalize(rawCode)

	sess, err := service.drive.Open(ctx)
	if err != nil {
		return domain.OfflineSubmission{}, fmt.Errorf("get 115 account for offline download: %w", err)
	}
	source := sess.Source()
	directory := source.Directory

	unlock, err := service.operations.Lock(ctx, source.AccountID, hash)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	defer unlock()

	existing, err := service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.HashEQ(hash),
		offlinedownload.StatusEQ(offlinedownload.StatusRunning)).First(ctx)
	if err == nil {
		if existing.DirectoryID != directory.ID {
			return domain.OfflineSubmission{}, domain.E(domain.KindConflict, "该磁力正在下载到另一个目录，请先在 115 中处理该任务", nil)
		}
		return service.submission(ctx, existing, &source)
	}
	if !ent.IsNotFound(err) {
		return domain.OfflineSubmission{}, fmt.Errorf("find active offline task: %w", err)
	}

	previous, err := service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.DirectoryIDEQ(directory.ID),
		offlinedownload.HashEQ(hash), offlinedownload.StatusEQ(offlinedownload.StatusDone)).
		Order(ent.Desc(offlinedownload.FieldID)).First(ctx)
	if err == nil {
		state, err := service.submission(ctx, previous, &source)
		if err != nil {
			return domain.OfflineSubmission{}, err
		}
		if state.Processing {
			return state, nil
		}
	} else if !ent.IsNotFound(err) {
		return domain.OfflineSubmission{}, fmt.Errorf("find download workflow: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return domain.OfflineSubmission{}, err
	}
	done, ok := service.drive.StartWork()
	if !ok {
		return domain.OfflineSubmission{}, context.Canceled
	}
	defer done()

	// Once a remote mutation starts, finish recording it even if the tab closes.
	submitContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), service.submitTimeout)
	defer cancel()
	remote, err := service.submit(submitContext, sess, hash)
	if err != nil {
		return domain.OfflineSubmission{}, fmt.Errorf("submit 115 offline download: %w", err)
	}
	var created *ent.OfflineDownload
	if err := service.drive.Commit(submitContext, func(tx *ent.Tx) error {
		var err error
		created, err = tx.OfflineDownload.Create().
			SetCode(code).SetJavdbID(movieID).SetHash(hash).SetInfoHash(remote.Hash).
			SetAccountID(source.AccountID).SetDirectoryID(directory.ID).
			Save(submitContext)
		if err != nil {
			return err
		}
		if remote.Status == 2 {
			return service.completeTask(submitContext, tx, created, remote.FileID)
		}
		return nil
	}); err != nil {
		return domain.OfflineSubmission{}, fmt.Errorf("record 115 offline download: %w", err)
	}

	service.tasks.NotifyOfflineChanged()
	created, err = service.database.OfflineDownload.Get(submitContext, created.ID)
	if err != nil {
		return domain.OfflineSubmission{}, err
	}
	return service.submission(submitContext, created, &source)
}
