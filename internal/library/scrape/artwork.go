package scrape

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"slices"

	"github.com/ppxb/miyabi/internal/domain"
	mediaimage "github.com/ppxb/miyabi/internal/image"
	"github.com/ppxb/miyabi/internal/nfo"
	"github.com/ppxb/miyabi/internal/tasks"
)

func (service *Service) prepareArtwork(ctx context.Context, taskID int, input *Payload) error {
	if input.Artwork == nil {
		body, selected, err := service.selectCover(ctx, input.Document.Images)
		if err != nil && ctx.Err() == nil && !slices.ContainsFunc(input.Document.Images, func(image domain.ImageCandidate) bool {
			return image.Provider == "javdb" && (image.Role == "cover" || image.Role == "poster")
		}) {
			fallback, fallbackErr := service.metadata.Fallback(ctx, domain.MovieRef{Code: input.Code, JavDBID: input.Document.JavDBID()})
			if fallbackErr != nil {
				return errors.Join(err, fallbackErr)
			}
			input.Document.Images = append(input.Document.Images, fallback.Images...)
			for _, source := range fallback.Detail.Sources {
				if !slices.ContainsFunc(input.Document.IDs, func(id nfo.UniqueID) bool { return id.Type == source.Provider && id.Value == source.ID }) {
					input.Document.IDs = append(input.Document.IDs, nfo.UniqueID{Type: source.Provider, Value: source.ID})
				}
			}
			body, selected, err = service.selectCover(ctx, fallback.Images)
		}
		if err != nil {
			return err
		}
		input.Document.SelectedImage = selected
		if input.Document.FieldSources == nil {
			input.Document.FieldSources = make(map[string]string)
		}
		input.Document.FieldSources["cover"] = selected.Provider
		if err := service.checkpointArtwork(ctx, taskID, input, body); err != nil {
			return err
		}
	} else if input.PosterVersion != mediaimage.PosterVersion {
		if input.Document.SelectedImage.Role == "poster" {
			input.PosterVersion = mediaimage.PosterVersion
			return nil
		}
		if err := service.checkpointArtwork(ctx, taskID, input, nil); err != nil {
			return err
		}
	}
	return nil
}

// Protect newly written images until their recovery checkpoint is durable.
// Cache maintenance retains artwork referenced by every unfinished scrape task.
func (service *Service) checkpointArtwork(ctx context.Context, taskID int, input *Payload, body []byte) error {
	if err := service.images.LockArtwork(ctx); err != nil {
		return err
	}
	defer service.images.UnlockArtwork()
	var artwork mediaimage.Artwork
	var err error
	if input.Artwork == nil {
		if input.Document.SelectedImage.Role == "poster" {
			artwork, err = service.images.Restore(body, body)
		} else {
			artwork, err = service.images.FromCover(body)
		}
	} else {
		artwork, err = service.images.RecropPoster(*input.Artwork)
	}
	if err != nil {
		return err
	}
	input.Artwork = &artwork
	input.PosterVersion = mediaimage.PosterVersion
	encoded, err := tasks.EncodePayload(input)
	if err != nil {
		return err
	}
	return service.db.Task.UpdateOneID(taskID).SetPayload(encoded).Exec(ctx)
}

// Compare effective poster pixels rather than compressed file size. Failed or
// corrupt images do not discard other confirmed candidates for the same film.
func (service *Service) selectCover(ctx context.Context, candidates []domain.ImageCandidate) ([]byte, domain.ImageCandidate, error) {
	var best []byte
	var selected domain.ImageCandidate
	var failures []error
	bestPixels := 0
	for _, fallback := range []bool{false, true} {
		for _, candidate := range candidates {
			if (candidate.Provider == "javdb") != fallback {
				continue
			}
			if candidate.Role != "cover" && candidate.Role != "poster" {
				continue
			}
			media, err := service.metadata.Image(ctx, candidate)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(media.Body))
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 80_000_000 {
				failures = append(failures, fmt.Errorf("invalid image dimensions"))
				continue
			}
			if _, _, err := image.Decode(bytes.NewReader(media.Body)); err != nil {
				failures = append(failures, err)
				continue
			}
			width := min(config.Width, config.Height*2/3)
			pixels := width * min(config.Height, config.Width*3/2)
			if candidate.Role == "poster" && width >= 600 {
				pixels *= 2
			}
			if pixels > bestPixels {
				best, selected, bestPixels = media.Body, candidate, pixels
			}
		}
		if len(best) > 0 {
			return best, selected, nil
		}
	}
	return nil, selected, domain.E(domain.KindUpstream, "所有封面候选均不可用", errors.Join(failures...))
}
