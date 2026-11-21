package catalogue

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent/offlinedownload"
	"github.com/ppxb/miyabi/internal/ent/task"
	"github.com/ppxb/miyabi/internal/tasks"
)

// MovieStates only reads the local index and tasks. Catalogue cache lifetimes
// and upstream availability must not delay admission badges.
func (service *Service) MovieStates(ctx context.Context, identities []MovieIdentity) ([]MovieStateItem, error) {
	result := make([]MovieStateItem, len(identities))
	if len(identities) == 0 {
		return result, nil
	}
	codes := make([]string, len(identities))
	for index, item := range identities {
		codes[index] = codeid.Normalize(item.Code)
		result[index] = MovieStateItem{ID: item.ID, State: MovieNotInLibrary}
	}
	if service.local == nil {
		return result, nil
	}
	localMovies, err := service.local.MatchingMovies(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("query local movie states: %w", err)
	}
	byCode := make(map[string]int)
	for _, record := range localMovies {
		byCode[record.Code] = record.ID
	}
	var taskIDs []any
	for index := range identities {
		item := &result[index]
		item.LibraryID = byCode[codes[index]]
		if item.LibraryID != 0 {
			item.State = MovieInLibrary
		} else {
			taskIDs = append(taskIDs, codes[index])
		}
	}
	if len(taskIDs) == 0 {
		return result, nil
	}

	var active []struct {
		Code string `json:"code"`
	}
	err = service.database.Task.Query().Where(
		task.TypeIn(tasks.KindScan.String(), tasks.KindScrape.String()), task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueIn(task.FieldPayload, taskIDs, sqljson.Path("code")),
			))
		}, func(s *sql.Selector) {
			// Cover payloads can contain full NFO documents; only read task identity.
			s.Select(sql.As(tasks.JSONExtract(s.C(task.FieldPayload), "code"), "code"))
		}).Select(task.FieldID).Scan(ctx, &active)
	if err != nil {
		return nil, fmt.Errorf("query movie workflows: %w", err)
	}
	saving, processing := make(map[string]bool), make(map[string]bool)
	for _, record := range active {
		processing[record.Code] = true
	}
	downloads, err := service.database.OfflineDownload.Query().Where(
		offlinedownload.CodeIn(codes...),
		offlinedownload.Or(offlinedownload.StatusEQ(offlinedownload.StatusRunning),
			offlinedownload.And(offlinedownload.StatusEQ(offlinedownload.StatusDone), offlinedownload.ScanTaskIDEQ(0),
				offlinedownload.Or(offlinedownload.FileIDNEQ(""), offlinedownload.AwaitingLocationEQ(true))))).
		Select(offlinedownload.FieldCode, offlinedownload.FieldStatus).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query movie downloads: %w", err)
	}
	for _, record := range downloads {
		if record.Status == offlinedownload.StatusRunning {
			saving[record.Code] = true
		} else {
			processing[record.Code] = true
		}
	}
	for index := range identities {
		item := &result[index]
		if item.LibraryID != 0 {
			continue
		}
		switch {
		case processing[codes[index]]:
			item.State = MovieProcessing
		case saving[codes[index]]:
			item.State = MovieSaving
		}
	}
	return result, nil
}
