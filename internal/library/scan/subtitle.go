package scan

import (
	"context"
	"maps"
	"slices"

	"github.com/ppxb/miyabi/internal/codeid"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/pan"
	subpkg "github.com/ppxb/miyabi/internal/subtitle"
)

// IndexDirectorySubtitles resolves and registers subtitle files found in the directory.
func IndexDirectorySubtitles(ctx context.Context, tx *ent.Tx, videos []Video, subtitles []pan.File) error {
	if len(videos) == 0 || len(subtitles) == 0 {
		return nil
	}

	// Map each video code to movie_id
	codes := make(map[string]int)
	for _, v := range videos {
		if v.Code != "" {
			codes[v.Code] = 0
		}
	}
	if len(codes) == 0 {
		return nil
	}

	matched, err := MatchMovies(ctx, tx, slices.Collect(maps.Keys(codes)))
	if err != nil {
		return err
	}
	maps.Copy(codes, matched)

	// If there is only one movie in this directory, it is an exclusive directory
	var exclusiveMovieID int
	if len(codes) == 1 {
		for _, id := range codes {
			exclusiveMovieID = id
		}
	}

	for _, sub := range subtitles {
		targetMovieID := 0
		subCode, _ := codeid.Parse(sub.Name)
		if subCode != "" {
			for c, id := range codes {
				if codeid.IsEquivalent(c, subCode) {
					targetMovieID = id
					break
				}
			}
			// If a distinct code was extracted but matches none of the videos in this directory,
			// it is an alien subtitle and must NOT fall back to exclusiveMovieID.
			if targetMovieID == 0 {
				continue
			}
		}
		if targetMovieID == 0 && exclusiveMovieID != 0 {
			targetMovieID = exclusiveMovieID
		}
		if targetMovieID == 0 {
			continue
		}

		if err := subpkg.IndexPanTrack(ctx, tx, targetMovieID, sub); err != nil {
			return err
		}
	}
	return nil
}
