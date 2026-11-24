package javdb

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/magnet"
)

// Magnets fetches the magnet list for a JavDB movie id. Callers treat any error
// as "no magnets from JavDB" and continue with the other sources.
func (c *Client) Magnets(ctx context.Context, movieID string) ([]domain.Magnet, error) {
	movieID = strings.TrimSpace(movieID)
	if movieID == "" {
		return nil, errors.New("JavDB movie ID is required")
	}
	var data wireMagnetsData
	if err := c.getJSON(ctx, "/api/v4/movies/"+url.PathEscape(movieID)+"/magnets", nil, &data); err != nil {
		return nil, err
	}
	return magnetsFromWire(data.Magnets), nil
}

func magnetsFromWire(items []wireMagnet) []domain.Magnet {
	magnets := make([]domain.Magnet, 0, len(items))
	for _, item := range items {
		hash := strings.ToLower(strings.TrimSpace(item.Hash))
		if hash == "" {
			continue
		}
		magnets = append(magnets, domain.Magnet{
			Hash:        hash,
			Name:        item.Name,
			Size:        item.SizeMiB << 20,
			HasSubtitle: item.CNSub,
			HD:          item.HD,
			FilesCount:  item.FilesCount,
			CreatedAt:   item.CreatedAt,
			Sources:     []string{domain.MagnetSourceJavDB},
		})
	}
	return magnets
}

// MagnetSource adapts the client to magnet.Source. It only queries when the
// caller supplies a JavDB id; otherwise it is skipped so the aggregator ignores
// it without counting a failure.
type MagnetSource struct{ client *Client }

func NewMagnetSource(client *Client) *MagnetSource { return &MagnetSource{client: client} }

func (*MagnetSource) Name() string { return domain.MagnetSourceJavDB }

func (s *MagnetSource) Find(ctx context.Context, ref domain.MovieRef) ([]domain.Magnet, error) {
	if strings.TrimSpace(ref.JavdbID) == "" {
		return nil, magnet.ErrSkipped
	}
	return s.client.Magnets(ctx, ref.JavdbID)
}
