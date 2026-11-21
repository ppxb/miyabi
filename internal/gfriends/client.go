package gfriends

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/syncx"
)

const (
	defaultFastlyURL = "https://fastly.jsdelivr.net/gh/gfriends/gfriends@master"
	defaultRawURL    = "https://raw.githubusercontent.com/gfriends/gfriends/master"
	cacheExpiration  = 7 * 24 * time.Hour
	indexRetryDelay  = 5 * time.Minute
	// Filetree.json is about 6.2 MiB as of October 2026; allow room for growth.
	maxIndexBytes = 32 << 20
)

// mirrors serve the same repository; the CDN is tried before GitHub.
var mirrors = []string{defaultFastlyURL, defaultRawURL}

type fileTree struct {
	Content map[string]map[string]string `json:"Content"`
}

type Client struct {
	dataDir    string
	httpClient *http.Client
	mu         sync.RWMutex
	allImages  map[string][]string // normalized name -> every relative path
	loadedAt   time.Time
	refresh    syncx.ContextLock
	retryAt    time.Time
	refreshErr error
}

// New uses the caller's HTTP client, including its timeout and proxy policy.
func New(dataDir string, httpClient *http.Client) *Client {
	return &Client{
		dataDir:    dataDir,
		httpClient: httpClient,
	}
}

// Lookup checks if an avatar exists for the given actor name.
func (c *Client) Lookup(name string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	norm := normalizeName(name)
	if rels := c.allImages[norm]; len(rels) > 0 {
		return rels[0], true
	}
	noSpace := strings.ReplaceAll(norm, " ", "")
	if rels := c.allImages[noSpace]; len(rels) > 0 {
		return rels[0], true
	}
	return "", false
}

// EnsureIndex loads the avatar index if needed. It is a thin alias for the
// unified loader that also backs multi-image lookups.
func (c *Client) EnsureIndex(ctx context.Context) error {
	_, err := c.ensureAll(ctx)
	return err
}

func readCachedAll(path string) (map[string][]string, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	if info.Size() > maxIndexBytes {
		return nil, time.Time{}, fmt.Errorf("gfriends index exceeds %d bytes", maxIndexBytes)
	}
	data, err := readLimited(file, maxIndexBytes)
	if err != nil {
		return nil, time.Time{}, err
	}
	all, err := parseIndex(data)
	return all, info.ModTime(), err
}

func parseIndex(data []byte) (map[string][]string, error) {
	var tree fileTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	all := buildAllIndex(tree)
	if len(all) == 0 {
		return nil, fmt.Errorf("gfriends filetree contains no usable avatars")
	}
	return all, nil
}

func (c *Client) installAll(all map[string][]string, updatedAt time.Time) {
	c.mu.Lock()
	c.allImages = all
	c.loadedAt = updatedAt
	c.retryAt = time.Time{}
	c.refreshErr = nil
	c.mu.Unlock()
}

// FetchAvatar downloads the avatar bytes for the specified actor.
func (c *Client) FetchAvatar(ctx context.Context, name string) ([]byte, error) {
	if err := c.EnsureIndex(ctx); err != nil {
		return nil, err
	}

	relPath, ok := c.Lookup(name)
	if !ok {
		return nil, os.ErrNotExist
	}

	// Encode path parts safely
	parts := strings.Split(relPath, "/")
	escapedParts := make([]string, len(parts))
	for i, part := range parts {
		if i == len(parts)-1 && strings.Contains(part, "?") {
			sub := strings.SplitN(part, "?", 2)
			escapedParts[i] = url.PathEscape(sub[0]) + "?" + sub[1]
		} else {
			escapedParts[i] = url.PathEscape(part)
		}
	}
	escapedPath := strings.Join(escapedParts, "/")

	data, err := c.download(ctx, escapedPath, 10<<20, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch avatar failed: %w", err)
	}
	return data, nil
}

// download tries the next mirror on transport, HTTP, read or validation failure.
func (c *Client) download(ctx context.Context, path string, limit int64, validate func([]byte) error) ([]byte, error) {
	var lastErr error
	for _, mirror := range mirrors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, mirror+"/"+path, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("download status: %d", resp.StatusCode)
			continue
		}
		if resp.ContentLength > limit {
			resp.Body.Close()
			lastErr = fmt.Errorf("gfriends download exceeds %d bytes", limit)
			continue
		}
		data, err := readLimited(resp.Body, limit)
		resp.Body.Close()
		if err == nil && validate != nil {
			err = validate(data)
		}
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, lastErr
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("gfriends data exceeds %d bytes", limit)
	}
	return data, nil
}

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s
}

// ensureAll loads and caches every file per normalized actor name. It is the
// single loader behind both avatar lookups and multi-image lookups: the on-disk
// Filetree cache is honored only while fresh, downloads are written back, and
// the parsed tree is built once.
func (c *Client) ensureAll(ctx context.Context) (map[string][]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if all, ready, err := c.readyImages(); ready {
		return all, err
	}
	if err := c.refresh.Lock(ctx); err != nil {
		return nil, err
	}
	defer c.refresh.Unlock()
	if all, ready, err := c.readyImages(); ready {
		return all, err
	}

	cacheFile := filepath.Join(c.dataDir, "gfriends_tree.json")
	c.mu.RLock()
	loaded := !c.loadedAt.IsZero()
	c.mu.RUnlock()
	if !loaded && c.dataDir != "" {
		if all, updatedAt, err := readCachedAll(cacheFile); err == nil {
			c.installAll(all, updatedAt)
			if images, ready, err := c.readyImages(); ready {
				return images, err
			}
		}
	}

	var all map[string][]string
	body, err := c.download(ctx, "Filetree.json", maxIndexBytes, func(data []byte) error {
		var err error
		all, err = parseIndex(data)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.mu.Lock()
		c.retryAt = time.Now().Add(indexRetryDelay)
		c.refreshErr = fmt.Errorf("download gfriends filetree: %w", err)
		failure := c.refreshErr
		existing, have := c.allImages, !c.loadedAt.IsZero()
		c.mu.Unlock()
		if have {
			return existing, nil
		}
		return nil, failure
	}
	if c.dataDir != "" {
		if err := os.MkdirAll(c.dataDir, 0755); err != nil {
			slog.WarnContext(ctx, "create gfriends cache directory", "path", c.dataDir, "error", err)
		} else if err := os.WriteFile(cacheFile, body, 0644); err != nil {
			slog.WarnContext(ctx, "write gfriends index cache", "path", cacheFile, "error", err)
		}
	}
	c.installAll(all, time.Now())
	images, _, _ := c.readyImages()
	return images, nil
}

// readyImages reports whether the in-memory images are fresh enough to serve,
// returning them when so (or the stale copy while a retry backoff is pending).
func (c *Client) readyImages() (map[string][]string, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loadedAt.IsZero() && time.Since(c.loadedAt) < cacheExpiration {
		return c.allImages, true, nil
	}
	if time.Now().Before(c.retryAt) {
		if !c.loadedAt.IsZero() {
			return c.allImages, true, nil
		}
		return nil, true, c.refreshErr
	}
	return nil, false, nil
}

func buildAllIndex(tree fileTree) map[string][]string {
	all := make(map[string][]string)
	for _, folder := range slices.Sorted(maps.Keys(tree.Content)) {
		for _, alias := range slices.Sorted(maps.Keys(tree.Content[folder])) {
			target := tree.Content[folder][alias]
			name := strings.TrimSuffix(strings.TrimSuffix(alias, ".jpg"), ".png")
			norm := normalizeName(name)
			if norm == "" || strings.TrimSpace(target) == "" {
				continue
			}
			rel := fmt.Sprintf("Content/%s/%s", folder, target)
			keys := []string{norm}
			if noSpace := strings.ReplaceAll(norm, " ", ""); noSpace != norm {
				keys = append(keys, noSpace)
			}
			for _, key := range keys {
				all[key] = append(all[key], rel)
			}
		}
	}
	return all
}

// Images returns every repository path for the given actor name (deduplicated,
// stable order). Empty when the actor is not in the gfriends dataset.
func (c *Client) Images(ctx context.Context, name string) ([]string, error) {
	all, err := c.ensureAll(ctx)
	if err != nil {
		return nil, err
	}
	norm := normalizeName(name)
	rels := append([]string{}, all[norm]...)
	if noSpace := strings.ReplaceAll(norm, " ", ""); noSpace != norm {
		rels = append(rels, all[noSpace]...)
	}
	seen := make(map[string]bool, len(rels))
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// FetchRel downloads one repository path (as returned by Images) as bytes.
func (c *Client) FetchRel(ctx context.Context, rel string) ([]byte, error) {
	return c.download(ctx, escapeContentRel(rel), 10<<20, nil)
}

func escapeContentRel(rel string) string {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if i == len(parts)-1 && strings.Contains(part, "?") {
			sub := strings.SplitN(part, "?", 2)
			parts[i] = url.PathEscape(sub[0]) + "?" + sub[1]
		} else {
			parts[i] = url.PathEscape(part)
		}
	}
	return strings.Join(parts, "/")
}
