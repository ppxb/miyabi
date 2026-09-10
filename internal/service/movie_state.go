package service

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/task"
)

type MovieIdentity struct {
	ID   string `json:"id" binding:"required,max=200"`
	Code string `json:"code" binding:"required,max=200"`
}

type DiscoverMovieState struct {
	ID        string     `json:"id"`
	LibraryID int        `json:"library_id,omitempty"`
	State     MovieState `json:"state"`
}

// MovieStates only reads the local index and tasks. Catalogue cache lifetimes
// and upstream availability must not delay admission badges or playback.
func (service *DiscoverService) MovieStates(ctx context.Context, identities []MovieIdentity) ([]DiscoverMovieState, error) {
	result := make([]DiscoverMovieState, len(identities))
	if len(identities) == 0 {
		return result, nil
	}
	ids := make([]string, len(identities))
	codes := make([]string, len(identities))
	for index, item := range identities {
		ids[index], codes[index] = item.ID, codeid.Normalize(item.Code)
		result[index] = DiscoverMovieState{ID: item.ID, State: MovieNotInLibrary}
	}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil || source == nil {
		return result, err
	}
	localMovies, err := service.database.Movie.Query().Where(
		movie.Or(movie.JavdbIDIn(ids...), movie.And(movie.JavdbIDIsNil(), movie.CodeIn(codes...))),
		movie.HasFilesWith(libraryFiles(*source)),
	).Select(movie.FieldID, movie.FieldCode, movie.FieldJavdbID).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query local movie states: %w", err)
	}
	byID, byCode := make(map[string]int), make(map[string]int)
	for _, record := range localMovies {
		if record.JavdbID != nil {
			byID[*record.JavdbID] = record.ID
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
	if len(taskIDs) == 0 {
		return result, nil
	}

	var active []struct {
		Type    string      `json:"type"`
		Status  task.Status `json:"status"`
		JavDBID string      `json:"javdb_id"`
	}
	err = service.database.Task.Query().Where(task.Or(
		task.And(task.TypeEQ("offline"), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueIn(task.FieldPayload, taskIDs, sqljson.Path("javdb_id")),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("directory_id")),
				sql.Or(sql.In(task.FieldStatus, string(task.StatusQueued), string(task.StatusRunning)),
					sql.And(sql.EQ(task.FieldStatus, string(task.StatusDone)),
						sql.Not(sqljson.HasKey(task.FieldPayload, sqljson.Path("scan_task_id"))),
						sqljson.ValueNEQ(task.FieldPayload, "", sqljson.Path("file_id")),
					)),
			))
		}),
		task.And(task.TypeIn("scan", "scrape", "cover"), task.StatusIn(task.StatusQueued, task.StatusRunning), func(s *sql.Selector) {
			s.Where(sql.And(
				sqljson.ValueIn(task.FieldPayload, taskIDs, sqljson.Path("javdb_id")),
				sqljson.ValueEQ(task.FieldPayload, source.AccountID, sqljson.Path("source", "account_id")),
				sqljson.ValueEQ(task.FieldPayload, source.Directory.ID, sqljson.Path("source", "directory", "id")),
			))
		}),
	), func(s *sql.Selector) {
		// Cover payloads can contain full NFO documents; only read task identity.
		s.Select(s.C(task.FieldType), s.C(task.FieldStatus),
			sql.As("json_extract("+s.C(task.FieldPayload)+", '$.javdb_id')", "javdb_id"))
	}).Select(task.FieldID).Scan(ctx, &active)
	if err != nil {
		return nil, fmt.Errorf("query movie workflows: %w", err)
	}
	saving, processing := make(map[string]bool), make(map[string]bool)
	for _, record := range active {
		if record.Type == "offline" && record.Status != task.StatusDone {
			saving[record.JavDBID] = true
		} else {
			processing[record.JavDBID] = true
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
