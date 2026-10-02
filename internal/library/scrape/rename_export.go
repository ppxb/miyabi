package scrape

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/movie"
)

// Remove only generated media entries after publishing the corrected identity.
// Original videos and other user files in the old directory remain untouched.
func (service *Service) removePreviousExport(ctx context.Context, root string, record *ent.Movie, code string) (string, error) {
	if root == "" || record.MetadataSnapshot == nil || record.MetadataSnapshot.Code == "" {
		return "", nil
	}
	oldCode := record.MetadataSnapshot.Code
	oldDir := EmbyMovieDir(root, oldCode)
	if strings.EqualFold(oldDir, EmbyMovieDir(root, code)) {
		return "", nil
	}
	claimed, err := service.db.Movie.Query().Where(movie.CodeEQ(oldCode), movie.IDNEQ(record.ID)).Exist(ctx)
	if err != nil || claimed {
		return "", err
	}
	entries, err := os.ReadDir(oldDir)
	if os.IsNotExist(err) {
		return oldDir, nil
	}
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || (ext != ".strm" && ext != ".nfo" && entry.Name() != "poster.jpg" && entry.Name() != "fanart.jpg") {
			continue
		}
		if err := os.Remove(filepath.Join(oldDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove previous export: %w", err)
		}
	}
	return oldDir, nil
}
