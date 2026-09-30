package catalogue

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/domain"
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
	ids := make([]string, len(identities))
	codes := make([]string, len(identities))
	for index, item := range identities {
		ids[index], codes[index] = item.ID, codeid.Normalize(item.Code)
		result[index] = MovieStateItem{ID: item.ID, State: MovieNotInLibrary}
	}
	var source *domain.LibrarySource
	if service.local != nil {
		source = service.local.Source()
	}
	if source == nil {
		return result, nil
	}
	localMovies, err := service.local.MatchingMovies(ctx, ids, codes)
	if err != nil {
		return nil, fmt.Errorf("query local movie states: %w", err)
	}
	byID, byCode := make(map[string]int), make(map[string]int)
	for _, record := range localMovies {
		if record.JavDBID != nil {
			byID[*record.JavDBID] = record.ID
		} else {
			byCode[record.Code] = record.ID
		}
	}
	var taskIDs []any
	for index, identity := range identities {
		item := &result[index]
		item.LibraryID = byID[identity.ID]
		if item.LibraryID == 0 {
			item.LibraryID = byCode[codes[index]]
		}
		if item.LibraryID != 0 {
			item.State = MovieInLibrary
		} else {
			taskIDs = append(taskIDs, identity.ID)
		}
	}
	if len(taskIDs) == 0 || service.database == nil {
		return result, nil
	}

	var active []struct {
		JavDBID string `json:"javdb_id"`
	}
	err = service.database.Task.Query().Where(
		task.TypeIn(tasks.KindScan.String(), tasks.KindScrape.String(), tasks.KindCover.String()), task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueIn(task.FieldPayload, taskIDs, sqljson.Path("javdb_id")),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("source", "account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("source", "directory", "id")),
			))
		}, func(s *sql.Selector) {
			// Cover payloads can contain full NFO documents; only read task identity.
			s.Select(sql.As(tasks.JSONExtract(s.C(task.FieldPayload), "javdb_id"), "javdb_id"))
		}).Select(task.FieldID).Scan(ctx, &active)
	if err != nil {
		return nil, fmt.Errorf("query movie workflows: %w", err)
	}
	saving, processing := make(map[string]bool), make(map[string]bool)
	for _, record := range active {
		processing[record.JavDBID] = true
	}
	downloads, err := service.database.OfflineDownload.Query().Where(
		offlinedownload.AccountIDEQ(source.AccountID), offlinedownload.DirectoryIDEQ(source.Directory.ID),
		offlinedownload.JavdbIDIn(ids...),
		offlinedownload.Or(offlinedownload.StatusEQ(offlinedownload.StatusRunning),
			offlinedownload.And(offlinedownload.StatusEQ(offlinedownload.StatusDone), offlinedownload.ScanTaskIDEQ(0),
				offlinedownload.Or(offlinedownload.FileIDNEQ(""), offlinedownload.AwaitingLocationEQ(true))))).
		Select(offlinedownload.FieldJavdbID, offlinedownload.FieldStatus).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query movie downloads: %w", err)
	}
	for _, record := range downloads {
		if record.Status == offlinedownload.StatusRunning {
			saving[record.JavdbID] = true
		} else {
			processing[record.JavdbID] = true
		}
	}
	for index, identity := range identities {
		item := &result[index]
		if item.LibraryID != 0 {
			continue
		}
		switch {
		case processing[identity.ID]:
			item.State = MovieProcessing
		case saving[identity.ID]:
			item.State = MovieSaving
		}
	}
	return result, nil
}
