package service

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/movie"
	"github.com/ppxb/miyabi/internal/ent/predicate"
	mediaimage "github.com/ppxb/miyabi/internal/image"
)

type LibrarySource struct {
	AccountID string              `json:"account_id"`
	Directory PanLibraryDirectory `json:"directory"`
}

type LibraryMovie struct {
	ID           int                `json:"id"`
	Code         string             `json:"code"`
	Title        string             `json:"title"`
	JavDBID      *string            `json:"javdb_id,omitempty"`
	Cover        *string            `json:"cover,omitempty"`
	Poster       *string            `json:"poster,omitempty"`
	ScrapeStatus movie.ScrapeStatus `json:"scrape_status"`
	FileCount    int                `json:"file_count"`
	Size         int64              `json:"size"`
}

type LibraryPage struct {
	Source         *LibrarySource `json:"source,omitempty"`
	Movies         []LibraryMovie `json:"movies"`
	Total          int            `json:"total"`
	FileCount      int            `json:"file_count"`
	UnmatchedFiles int            `json:"unmatched_files"`
	Page           int            `json:"page"`
	HasMore        bool           `json:"has_more"`
}

type LibraryFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type LibraryFilePage struct {
	Files   []LibraryFile `json:"files"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	HasMore bool          `json:"has_more"`
}

type LibraryService struct {
	images       *mediaimage.Cache
	database     *ent.Client
	drive        *PanService
	tasks        *TaskService
	minVideoSize int64
}

func NewLibraryService(database *ent.Client, drive *PanService, tasks *TaskService, images *mediaimage.Cache, minVideoSize int64) *LibraryService {
	return &LibraryService{database: database, drive: drive, tasks: tasks, images: images, minVideoSize: minVideoSize}
}

// Browsing an existing index only reads SQLite. 115 is contacted when scanning,
// not on each visit to the library or while paging through indexed files.
func loadLibrarySource(ctx context.Context, database *ent.Client) (*LibrarySource, error) {
	directory, found, err := loadSetting[panLibraryDirectory](ctx, database, panDirectorySetting)
	if err != nil || !found {
		return nil, err
	}
	return &LibrarySource{AccountID: directory.AccountID, Directory: directory.PanLibraryDirectory}, nil
}

func libraryFiles(source LibrarySource) predicate.File {
	return file.And(file.AccountIDEQ(source.AccountID), file.RootIDEQ(source.Directory.ID))
}

func (service *LibraryService) Movies(ctx context.Context, page, limit int) (LibraryPage, error) {
	result := LibraryPage{Movies: []LibraryMovie{}, Page: page}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return result, err
	}
	if source == nil {
		return result, nil
	}
	result.Source = source
	scope := libraryFiles(*source)
	query := service.database.Movie.Query().Where(movie.HasFilesWith(scope))
	result.Total, err = query.Clone().Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count library movies: %w", err)
	}
	result.FileCount, err = service.database.File.Query().Where(scope).Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count library files: %w", err)
	}
	result.UnmatchedFiles, err = service.database.File.Query().Where(scope, file.Not(file.HasMovie())).Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count unidentified videos: %w", err)
	}
	records, err := query.Order(ent.Desc(movie.FieldCreatedAt), ent.Desc(movie.FieldID)).
		Offset((page - 1) * limit).Limit(limit).
		WithFiles(func(query *ent.FileQuery) { query.Where(scope).Select(file.FieldID, file.FieldSize) }).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list library movies: %w", err)
	}
	for _, record := range records {
		item := LibraryMovie{
			ID: record.ID, Code: record.Code, Title: record.Title, ScrapeStatus: record.ScrapeStatus,
			JavDBID: record.JavdbID, Cover: record.Cover, Poster: record.Poster,
			FileCount: len(record.Edges.Files),
		}
		for _, media := range record.Edges.Files {
			item.Size += media.Size
		}
		result.Movies = append(result.Movies, item)
	}
	result.HasMore = (page-1)*limit+len(result.Movies) < result.Total
	return result, nil
}

func (service *LibraryService) Files(ctx context.Context, movieID int, unmatched bool, page, limit int) (LibraryFilePage, error) {
	result := LibraryFilePage{Files: []LibraryFile{}, Page: page}
	source, err := loadLibrarySource(ctx, service.database)
	if err != nil {
		return result, err
	}
	if source == nil {
		return result, nil
	}
	query := service.database.File.Query().Where(libraryFiles(*source))
	if movieID != 0 {
		query.Where(file.MovieIDEQ(movieID))
	}
	if unmatched {
		query.Where(file.Not(file.HasMovie()))
	}
	result.Total, err = query.Clone().Count(ctx)
	if err != nil {
		return result, fmt.Errorf("count indexed files: %w", err)
	}
	records, err := query.Order(ent.Asc(file.FieldPath), ent.Asc(file.FieldID)).
		Offset((page - 1) * limit).Limit(limit).All(ctx)
	if err != nil {
		return result, fmt.Errorf("list indexed files: %w", err)
	}
	for _, record := range records {
		result.Files = append(result.Files, LibraryFile{
			ID: record.FileID, Name: record.Name, Path: record.Path, Size: record.Size,
		})
	}
	result.HasMore = (page-1)*limit+len(result.Files) < result.Total
	return result, nil
}
