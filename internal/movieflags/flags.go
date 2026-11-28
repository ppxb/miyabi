package movieflags

import (
	"context"
	"fmt"

	"github.com/ppxb/miyabi/internal/domain"
	subtitlemeta "github.com/ppxb/miyabi/internal/domain/subtitle"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/ent/subtitle"
	"github.com/ppxb/miyabi/internal/magnet"
)

// Flags holds the per-movie display flags derived from indexed files and online
// subtitle tracks.
type Flags struct {
	Source        map[int]string
	HasSubtitle   map[int]bool
	HasUncensored map[int]bool
	HasCracked    map[int]bool
}

// Load derives flags for the given movies. providers maps cloud account IDs to
// the source badge they show; mounted contains the account/directory pairs that
// are currently mounted. Files outside a mounted source set no source badge.
func Load(ctx context.Context, db *ent.Client, movieIDs []int, providers map[string]string, mounted map[[2]string]bool) (Flags, error) {
	flags := Flags{
		Source:        make(map[int]string, len(movieIDs)),
		HasSubtitle:   make(map[int]bool, len(movieIDs)),
		HasUncensored: make(map[int]bool, len(movieIDs)),
		HasCracked:    make(map[int]bool, len(movieIDs)),
	}
	if len(movieIDs) == 0 {
		return flags, nil
	}
	records, err := db.File.Query().
		Where(file.MovieIDIn(movieIDs...)).
		Select(file.FieldMovieID, file.FieldName, file.FieldAccountID, file.FieldRootID).All(ctx)
	if err != nil {
		return Flags{}, fmt.Errorf("read media files: %w", err)
	}
	for _, record := range records {
		if record.MovieID == nil {
			continue
		}
		id := *record.MovieID
		if record.AccountID != "" && record.AccountID != domain.LocalAccountID &&
			mounted[[2]string{record.AccountID, record.RootID}] {
			provider := providers[record.AccountID]
			if provider == "" {
				provider = "115"
			}
			if flags.Source[id] != "guangya" {
				flags.Source[id] = provider
			}
		}
		if subtitlemeta.HasChineseSubtitle(record.Name) {
			flags.HasSubtitle[id] = true
		}
		inferred := magnet.Infer(record.Name)
		if inferred.HasUncensored {
			flags.HasUncensored[id] = true
		}
		if inferred.HasCracked {
			flags.HasCracked[id] = true
		}
	}
	tracks, err := db.Subtitle.Query().
		Where(subtitle.MovieIDIn(movieIDs...)).
		Select(subtitle.FieldMovieID, subtitle.FieldName).All(ctx)
	if err != nil {
		return Flags{}, fmt.Errorf("read media subtitles: %w", err)
	}
	for _, track := range tracks {
		if subtitlemeta.HasChineseSubtitle(track.Name) {
			flags.HasSubtitle[track.MovieID] = true
		}
	}
	return flags, nil
}
