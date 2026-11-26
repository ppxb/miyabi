package scan

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/database"
	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/library/scrape"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

// RescrapeMovie queues a fresh metadata workflow for the indexed files of one movie.
// A supplied code persists the user's correction before the recoverable job starts.
func (s *Scanner) RescrapeMovie(ctx context.Context, id int, code string) (*ent.Task, error) {
	if code != "" && !codeid.Valid(code) {
		return nil, domain.E(domain.KindInvalid, "请输入有效的影片番号", nil)
	}
	code = codeid.Normalize(code)
	source, err := s.movieSource(ctx, id)
	if err != nil {
		return nil, err
	}
	sess, err := s.driveSvc.OpenSource(ctx, source)
	if err != nil {
		return nil, err
	}
	var parent *ent.Task
	// Mount callbacks enqueue with the source lock held. Keep the same order,
	// and acquire the queue before opening a database write transaction.
	err = sess.WithSource(ctx, func() error {
		if err := s.tasksSvc.Queue().Lock(ctx); err != nil {
			return err
		}
		defer s.tasksSvc.Queue().Unlock()
		return ent.WithTx(ctx, s.db, func(tx *ent.Tx) error {
			record, err := tx.Movie.Query().Where(movie.IDEQ(id), movie.HasFilesWith(database.LibraryFiles(sess.Source()))).Only(ctx)
			if ent.IsNotFound(err) {
				return domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
			}
			if err != nil {
				return err
			}
			correcting := code != "" && code != record.Code
			policy := ManualScrapePolicy
			if correcting {
				policy = ConflictScrapePolicy
			}
			plan, err := PlanScrapes(ctx, tx, []ScrapeJob{{
				MovieID: id, Rebuild: true, Source: sess.Source(),
			}}, policy)
			if err != nil {
				return err
			}
			if active, reused := plan.Reused[id]; reused {
				parent, err = tx.Task.Get(ctx, active.ScanTaskID)
				return err
			}
			busy, err := tx.Task.Query().Where(task.TypeEQ(tasks.KindScan.String()),
				task.StatusIn(task.StatusQueued, task.StatusRunning), func(q *sql.Selector) {
					q.Where(sqljson.ValueEQ(task.FieldPayload, sess.Source().AccountID, sqljson.Path("source", "account_id")))
					q.Where(sqljson.ValueEQ(task.FieldPayload, sess.Source().Directory.ID, sqljson.Path("source", "directory", "id")))
				}).Exist(ctx)
			if err != nil {
				return err
			}
			if busy {
				return domain.E(domain.KindConflict, "媒体库正在扫描，请完成后再操作", nil)
			}
			if correcting {
				matches, err := MatchMovies(ctx, tx, []string{code})
				if err != nil {
					return err
				}
				if other := matches[code]; other != 0 && other != id {
					return domain.E(domain.KindConflict, "该番号已存在于媒体库，请检查后重试", nil)
				}
				if err := scrape.SaveMovieMetadata(ctx, tx, id, nfo.Movie{Code: code}); err != nil {
					return err
				}
				update := tx.Movie.UpdateOneID(id).SetManualCode(code).ClearCover().ClearPoster().SetFanarts([]string{})
				if record.MetadataSnapshot != nil {
					snapshot := *record.MetadataSnapshot
					if snapshot.Code == "" {
						snapshot.Code = record.Code
					}
					update.SetMetadataSnapshot(&snapshot)
				}
				record, err = update.Save(ctx)
				if err != nil {
					return err
				}
			} else if err := tx.Movie.UpdateOneID(id).SetScrapeStatus(movie.ScrapeStatusPending).Exec(ctx); err != nil {
				return err
			}
			body, err := tasks.EncodePayload(domain.ScanPayload{
				MovieID: id, Code: record.Code, Rebuild: true, Source: sess.Source(),
				Scan: domain.ScanProgress{Stage: "done", Movies: 1, CurrentPath: sess.Source().Directory.Path},
			})
			if err != nil {
				return err
			}
			parent, err = tx.Task.Create().SetType(tasks.KindScan.String()).SetStatus(task.StatusDone).SetPayload(body).Save(ctx)
			if err != nil {
				return err
			}
			create, err := NewScrapeTask(tx, ScrapeJob{
				Rebuild: true, Source: sess.Source(), ScanTaskID: parent.ID, MovieID: id,
				Code: record.Code, ManualCode: record.ManualCode,
			})
			if err != nil {
				return err
			}
			return create.Exec(ctx)
		})
	})
	if err != nil {
		return nil, err
	}
	s.tasksSvc.NotifyLibraryChanged()
	s.tasksSvc.WakePool()
	return parent, nil
}

// movieSource resolves the 115 source whose current mount holds the movie's files.
func (s *Scanner) movieSource(ctx context.Context, id int) (domain.LibrarySource, error) {
	record, err := s.db.File.Query().
		Where(file.MovieIDEQ(id), file.AccountIDNEQ(domain.LocalAccountID)).
		Select(file.FieldAccountID).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
		}
		return domain.LibrarySource{}, err
	}
	source, ok, err := s.driveSvc.Mount(ctx, record.AccountID)
	if err != nil {
		return domain.LibrarySource{}, err
	}
	if !ok {
		return domain.LibrarySource{}, domain.E(domain.KindNotFound, "当前挂载目录中未找到该影片的媒体文件", nil)
	}
	return source, nil
}
