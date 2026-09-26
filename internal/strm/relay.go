// Package strm resolves the fixed URLs written into exported .strm files to
// short-lived 115 media URLs, so Emby clients stream directly from the 115 CDN.
package strm

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/pan"
)

// originalDefinition marks the untranscoded source among 115 playback streams.
const originalDefinition = 100

// Relay exchanges indexed 115 video IDs for playable media URLs.
type Relay struct {
	database *ent.Client
	drive    *drive.Drive
}

func New(database *ent.Client, d *drive.Drive) *Relay {
	return &Relay{database: database, drive: d}
}

// StreamURL returns the best 115 stream for a video: the original quality,
// otherwise the highest transcoded resolution. 115 signs these URLs for a
// short time, so every request resolves a fresh one.
func (relay *Relay) StreamURL(ctx context.Context, fileID string) (string, error) {
	if relay.drive == nil {
		return "", drive.ErrMediaDirectoryRequired
	}
	sess, err := relay.drive.Open(ctx)
	if err != nil {
		return "", err
	}
	pickCode, err := relay.pickCode(ctx, sess, fileID)
	if err != nil {
		return "", err
	}
	sources, err := sess.PlayURL(ctx, pickCode)
	if err != nil {
		// pan reports the raw condition; the user-facing text lives in drive.
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return "", drive.ErrTranscodeUnavailable
		}
		return "", fmt.Errorf("get 115 playback URL: %w", err)
	}
	var best string
	bestScore := -1
	for _, source := range sources {
		score := source.Height
		if source.Definition == originalDefinition {
			score += 1 << 20
		}
		if source.URL != "" && score > bestScore {
			best, bestScore = source.URL, score
		}
	}
	if best == "" {
		return "", domain.E(domain.KindNotFound, "115 未返回有效视频播放流", nil)
	}
	return best, nil
}

// Probe forwards a HEAD request to the 115 CDN so media servers can read
// stream metadata without following the redirect.
func (relay *Relay) Probe(ctx context.Context, address string, headers http.Header) (*http.Response, error) {
	if relay.drive == nil {
		return nil, drive.ErrMediaDirectoryRequired
	}
	return relay.drive.OpenMedia(ctx, http.MethodHead, address, headers)
}

// pickCode prefers the indexed pick code and asks 115 only for videos indexed
// before pick codes were stored.
func (relay *Relay) pickCode(ctx context.Context, sess drive.Session, fileID string) (string, error) {
	record, err := relay.database.File.Query().Where(file.FileIDEQ(fileID)).Select(file.FieldPickCode).First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return "", fmt.Errorf("read indexed video: %w", err)
	}
	if record != nil && record.PickCode != "" {
		return record.PickCode, nil
	}
	info, err := sess.Info(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("read 115 video info: %w", err)
	}
	if info.PickCode == "" {
		return "", domain.E(domain.KindNotFound, "未能获取到视频的 115 提取码", nil)
	}
	return info.PickCode, nil
}
