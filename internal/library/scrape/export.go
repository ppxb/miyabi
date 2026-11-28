package scrape

import (
	"context"

	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/export"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/pan"
)

// ExportLocalMovie repairs missing or changed outputs using saved metadata and artwork.
func ExportLocalMovie(ctx context.Context, db *ent.Client, embyDir, publicURL, strmToken string, record *ent.Movie, images *mediaimage.Cache) (bool, error) {
	if record.Code == "" {
		return false, nil
	}

	doc, err := MovieNFO(record)
	if err != nil {
		return false, err
	}
	doc, err = canonicalExportDocument(ctx, db, record.ID, doc)
	if err != nil {
		return false, err
	}
	videos := make([]pan.File, 0, len(record.Edges.Files))
	for _, f := range record.Edges.Files {
		videos = append(videos, pan.File{ID: f.FileID, Name: f.Name, Size: f.Size, PickCode: f.PickCode})
	}
	var posterBytes, fanartBytes []byte
	artwork := MovieArtwork(record)
	if artwork.Poster != "" {
		posterBytes, err = images.ReadURL(artwork.Poster)
		if err != nil {
			return false, err
		}
	}
	if artwork.Fanart != "" {
		fanartBytes, err = images.ReadURL(artwork.Fanart)
	} else if artwork.Thumbnail != "" {
		fanartBytes, err = images.ReadURL(artwork.Thumbnail)
	}
	if err != nil {
		return false, err
	}
	return export.WriteMedia(export.Config{EmbyDir: embyDir, PublicURL: publicURL, STRMToken: strmToken}, record.Code, doc, videos, posterBytes, fanartBytes)
}
