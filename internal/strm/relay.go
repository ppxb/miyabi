// Package strm resolves the fixed URLs written into exported .strm files to
// short-lived 115 media URLs, so Emby clients stream directly from the 115 CDN.
package strm

import (
	"context"
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

// New requires an initialized database and drive.
func New(database *ent.Client, d *drive.Drive) *Relay {
	return &Relay{database: database, drive: d}
}

// StreamURL shares short-lived signed URLs within the same source, authorization
// and user agent, and verifies the session including on cache hits.
func (relay *Relay) StreamURL(ctx context.Context, fileID, userAgent string) (string, error) {
	sess, source, pickCode, err := relay.sessionForFile(ctx, fileID)
	if err != nil {
		return "", err
	}
	media, err := relay.drive.ResolveMedia(ctx, sess, source.AccountID, fileID, pickCode, userAgent)
	if err != nil {
		return "", err
	}
	if media.DownloadURL != "" {
		return media.DownloadURL, nil
	}
	return bestStream(media.Streams), nil
}

// bestStream prefers the original stream, then the highest transcoded resolution.
func bestStream(sources []pan.PlaySource) string {
	var best string
	bestScore := -1
	for _, source := range sources {
		score := source.Height
		if source.Definition == originalDefinition {
			score += 1 << 20
		}
		if score > bestScore {
			best, bestScore = source.URL, score
		}
	}
	return best
}

// Probe forwards a HEAD request to the 115 CDN so media servers can read
// stream metadata without following the redirect.
func (relay *Relay) Probe(ctx context.Context, address string, headers http.Header) (*http.Response, error) {
	return relay.drive.OpenMedia(ctx, http.MethodHead, address, headers)
}

// sessionForFile opens the session for the 115 account that owns the indexed
// video, so STRM streams also work for files from any account.
func (relay *Relay) sessionForFile(ctx context.Context, fileID string) (drive.Session, domain.LibrarySource, string, error) {
	record, err := relay.database.File.Query().Where(file.FileIDEQ(fileID)).
		Select(file.FieldAccountID, file.FieldRootID, file.FieldPickCode).First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, domain.LibrarySource{}, "", domain.E(domain.KindNotFound, "视频索引不存在，请重新扫描媒体库", err)
		}
		return nil, domain.LibrarySource{}, "", fmt.Errorf("read indexed video: %w", err)
	}
	source, ok, err := relay.drive.Mount(ctx, record.AccountID)
	if err != nil {
		return nil, domain.LibrarySource{}, "", err
	}
	if !ok || source.Directory.ID != record.RootID {
		return nil, domain.LibrarySource{}, "", drive.ErrSourceChanged
	}
	sess, err := relay.drive.OpenSource(ctx, source)
	if err != nil {
		return nil, domain.LibrarySource{}, "", err
	}
	return sess, source, record.PickCode, nil
}
