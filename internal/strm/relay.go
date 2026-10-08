// Package strm resolves the fixed URLs written into exported .strm files to
// short-lived 115 media URLs, so Emby clients stream directly from the 115 CDN.
package strm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/drive"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/ent/file"
	"github.com/ppxb/miyabi/internal/pan"
	"golang.org/x/sync/singleflight"
)

// originalDefinition marks the untranscoded source among 115 playback streams.
const originalDefinition = 100

// Relay exchanges indexed 115 video IDs for playable media URLs.
type Relay struct {
	database  *ent.Client
	drive     *drive.Drive
	resolving singleflight.Group
	cacheMu   sync.Mutex
	urls      map[string]cachedURL
}

// New requires an initialized database and drive.
func New(database *ent.Client, d *drive.Drive) *Relay {
	return &Relay{database: database, drive: d}
}

// StreamURL shares short-lived signed URLs within the same source, authorization
// and user agent. Each caller still verifies its session, including on cache hits.
func (relay *Relay) StreamURL(ctx context.Context, fileID, userAgent string) (string, error) {
	ua := strings.TrimSpace(userAgent)
	sess, err := relay.drive.Open(ctx)
	if err != nil {
		return "", err
	}
	source := sess.Source()
	key := fmt.Sprintf("%d:%q:%q:%q:%q", sess.AuthorizationVersion(), source.AccountID, source.Directory.ID, fileID, ua)
	result := relay.resolving.DoChan(key, func() (any, error) {
		done, ok := relay.drive.StartWork()
		if !ok {
			return "", context.Canceled
		}
		defer done()
		// One player's canceled probe must not cancel another player's lookup.
		resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		if address := relay.cached(key, time.Now()); address != "" {
			return address, nil
		}
		address, err := relay.resolveURL(resolveCtx, sess, fileID, ua)
		if err == nil {
			relay.remember(key, address, time.Now())
		}
		return address, err
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return "", completed.Err
		}
		var address string
		err := sess.WithSource(ctx, func() error {
			address = completed.Val.(string)
			return nil
		})
		return address, err
	}
}

// resolveURL prefers a direct download URL, then the original stream or highest
// transcoded resolution. The indexed pick code is read only on cache misses.
func (relay *Relay) resolveURL(ctx context.Context, sess drive.Session, fileID, ua string) (string, error) {
	pickCode, err := relay.pickCode(ctx, sess, fileID)
	if err != nil {
		return "", err
	}
	// Prefer the direct download URL for full CDN throughput and instant seeking.
	downloadURL, err := sess.DownloadURL(ctx, pickCode, ua)
	if err == nil {
		return downloadURL, nil
	}
	if !errors.Is(err, pan.ErrDownloadUnavailable) {
		return "", err
	}
	sources, err := sess.PlayURL(ctx, pickCode, ua)
	if err != nil {
		// pan reports the raw condition; the user-facing text lives in drive.
		if errors.Is(err, pan.ErrTranscodeUnavailable) {
			return "", drive.ErrTranscodeUnavailable
		}
		return "", fmt.Errorf("get 115 playback URL: %w", err)
	}
	// pan has validated a nonempty list of URLs with positive heights.
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
	return best, nil
}

// Probe forwards a HEAD request to the 115 CDN so media servers can read
// stream metadata without following the redirect.
func (relay *Relay) Probe(ctx context.Context, address string, headers http.Header) (*http.Response, error) {
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
