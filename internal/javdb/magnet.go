package javdb

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Magnets returns subtitle and HD resources first, then orders by size.
func (c *Client) Magnets(ctx context.Context, movieID string) ([]Magnet, error) {
	movieID = strings.TrimSpace(movieID)
	if movieID == "" {
		return nil, errors.New("JavDB movie ID is required")
	}
	var data wireMagnetsData
	if err := c.getJSON(ctx, "/api/v1/movies/"+url.PathEscape(movieID)+"/magnets", nil, defaultLanguage, &data); err != nil {
		return nil, err
	}
	magnets := make([]Magnet, len(data.Magnets))
	for index, item := range data.Magnets {
		hash, err := hex.DecodeString(item.Hash)
		if err != nil {
			return nil, fmt.Errorf("decode JavDB magnet %d hash: %w", index, err)
		}
		if len(hash) != 20 {
			return nil, fmt.Errorf("JavDB magnet %d has an invalid info hash length", index)
		}
		magnets[index] = Magnet{
			Hash: hex.EncodeToString(hash), Name: item.Name, Size: item.SizeMiB * 1024 * 1024,
			HasSubtitle: item.CNSub, HD: item.HD, FilesCount: item.FilesCount, CreatedAt: item.CreatedAt,
		}
	}
	slices.SortStableFunc(magnets, func(a, b Magnet) int {
		if a.HasSubtitle != b.HasSubtitle {
			if a.HasSubtitle {
				return -1
			}
			return 1
		}
		if a.HD != b.HD {
			if a.HD {
				return -1
			}
			return 1
		}
		if a.Size != b.Size {
			return cmp.Compare(b.Size, a.Size)
		}
		return cmp.Compare(b.FilesCount, a.FilesCount)
	})
	return magnets, nil
}
