package emby

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/embynotification"
)

func (s *Service) NotifyUpdated(ctx context.Context, path string) error {
	return s.enqueueNotification(ctx, s.db, path)
}

// NotifyUpdatedTx commits the notification with the caller's library changes.
func (s *Service) NotifyUpdatedTx(ctx context.Context, tx *ent.Tx, path string) error {
	return s.enqueueNotification(ctx, tx.Client(), path)
}

func (s *Service) enqueueNotification(ctx context.Context, db *ent.Client, path string) error {
	if !s.currentConfig().ready() || strings.TrimSpace(path) == "" {
		return nil
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve Emby notification path: %w", err)
	}
	now := time.Now()
	return db.EmbyNotification.Create().SetPath(path).SetNextAttemptAt(now).
		OnConflictColumns(embynotification.FieldPath).
		Update(func(update *ent.EmbyNotificationUpsert) {
			update.AddRevision(1).SetAttempts(0).SetLastError("").SetNextAttemptAt(now).SetUpdatedAt(now)
		}).Exec(ctx)
}

// RetryPending bypasses backoff once, including when a manual library scan starts.
func (s *Service) RetryPending(ctx context.Context) error {
	if err := s.db.EmbyNotification.Update().AddRevision(1).SetNextAttemptAt(time.Now()).Exec(ctx); err != nil {
		return err
	}
	select {
	case s.wakeNotifications <- struct{}{}:
	default:
	}
	return nil
}

func notificationBackoff(attempts int) time.Duration {
	return min(15*time.Second*time.Duration(1<<min(attempts, 5)), 5*time.Minute)
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			s.flushNotifications(flushCtx)
			cancel()
			return
		case <-ticker.C:
			s.flushNotifications(ctx)
		case <-s.wakeNotifications:
			s.flushNotifications(ctx)
		}
	}
}

func (s *Service) flushNotifications(ctx context.Context) {
	cfg := s.currentConfig()
	if !cfg.ready() {
		return
	}
	rows, err := s.db.EmbyNotification.Query().Where(embynotification.NextAttemptAtLTE(time.Now())).
		Order(ent.Asc(embynotification.FieldNextAttemptAt), ent.Asc(embynotification.FieldID)).Limit(50).All(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "load Emby notifications", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	var valid []*ent.EmbyNotification
	var paths []string
	for _, row := range rows {
		if translatePath(row.Path, cfg.LocalDir, cfg.MediaPath) == "" {
			err := domain.E(domain.KindInvalid, "待通知目录与 Emby 路径配置不匹配", nil)
			if saveErr := s.finishNotifications(ctx, []*ent.EmbyNotification{row}, err); saveErr != nil {
				slog.ErrorContext(ctx, "save Emby notification failure", "error", saveErr)
			}
			continue
		}
		valid = append(valid, row)
		paths = append(paths, row.Path)
	}
	if len(valid) == 0 {
		return
	}
	err = s.sendBatch(ctx, cfg, paths)
	if saveErr := s.finishNotifications(ctx, valid, err); saveErr != nil {
		slog.ErrorContext(ctx, "save Emby notification result", "error", saveErr)
	}
	if err != nil {
		slog.WarnContext(ctx, "Emby notification failed; retained for retry", "count", len(valid), "error", err)
	} else {
		s.actors.schedule(25 * time.Second)
	}
}

func (s *Service) finishNotifications(ctx context.Context, rows []*ent.EmbyNotification, result error) error {
	return ent.WithTx(ctx, s.db, func(tx *ent.Tx) error {
		for _, row := range rows {
			// An update arriving during the request owns a newer revision and must survive.
			match := embynotification.And(embynotification.IDEQ(row.ID), embynotification.RevisionEQ(row.Revision))
			if result == nil {
				if _, err := tx.EmbyNotification.Delete().Where(match).Exec(ctx); err != nil {
					return err
				}
			} else {
				if err := tx.EmbyNotification.Update().Where(match).AddAttempts(1).
					SetNextAttemptAt(time.Now().Add(notificationBackoff(row.Attempts))).SetLastError(domain.PublicMessage(result)).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
