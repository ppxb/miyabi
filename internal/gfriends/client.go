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
	index      map[string]string // normalized name -> relative path e.g. "Content/9-Javrave/xxx.jpg?t=..."
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
		index:      make(map[string]string),
	}
}

// Lookup checks if an avatar exists for the given actor name.
func (c *Client) Lookup(name string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	norm := normalizeName(name)
	if rel, ok := c.index[norm]; ok {
		return rel, true
	}
	noSpace := strings.ReplaceAll(norm, " ", "")
	if rel, ok := c.index[noSpace]; ok {
		return rel, true
	}
	return "", false
}

// EnsureIndex serializes refreshes without blocking readers of the current index.
func (c *Client) EnsureIndex(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ready, err := c.indexReady(); ready {
		return err
	}
	if err := c.refresh.Lock(ctx); err != nil {
		return err
	}
	defer c.refresh.Unlock()
	if ready, err := c.indexReady(); ready {
		return err
	}

	cacheFile := filepath.Join(c.dataDir, "gfriends_tree.json")
	c.mu.RLock()
	loaded := !c.loadedAt.IsZero()
	c.mu.RUnlock()
	if !loaded && c.dataDir != "" {
		if index, updatedAt, err := readCachedIndex(cacheFile); err == nil {
			c.installIndex(index, updatedAt)
			if ready, err := c.indexReady(); ready {
				return err
			}
		}
	}

	var index map[string]string
	body, err := c.download(ctx, "Filetree.json", 0, func(data []byte) error {
		var err error
		index, err = parseIndex(data)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		c.retryAt = time.Now().Add(indexRetryDelay)
		c.refreshErr = fmt.Errorf("download gfriends filetree: %w", err)
		if !c.loadedAt.IsZero() {
			return nil
		}
		return c.refreshErr
	}
	if c.dataDir != "" {
		if err := os.MkdirAll(c.dataDir, 0755); err != nil {
			slog.WarnContext(ctx, "create gfriends cache directory", "path", c.dataDir, "error", err)
		} else if err := os.WriteFile(cacheFile, body, 0644); err != nil {
			slog.WarnContext(ctx, "write gfriends index cache", "path", cacheFile, "error", err)
		}
	}
	c.installIndex(index, time.Now())
	return nil
}

func readCachedIndex(path string) (map[string]string, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	index, err := parseIndex(data)
	return index, info.ModTime(), err
}

func parseIndex(data []byte) (map[string]string, error) {
	var tree fileTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	index := buildIndex(tree)
	if len(index) == 0 {
		return nil, fmt.Errorf("gfriends filetree contains no usable avatars")
	}
	return index, nil
}

func (c *Client) indexReady() (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loadedAt.IsZero() && time.Since(c.loadedAt) < cacheExpiration {
		return true, nil
	}
	if time.Now().Before(c.retryAt) {
		if !c.loadedAt.IsZero() {
			return true, nil
		}
		return true, c.refreshErr
	}
	return false, nil
}

func (c *Client) installIndex(index map[string]string, updatedAt time.Time) {
	c.mu.Lock()
	c.index = index
	c.loadedAt = updatedAt
	c.retryAt = time.Time{}
	c.refreshErr = nil
	c.mu.Unlock()
}

// buildIndex indexes folders in name order and keeps the first image per
// actor, so the chosen avatar does not depend on map iteration order.
func buildIndex(tree fileTree) map[string]string {
	index := make(map[string]string)
	for _, folder := range slices.Sorted(maps.Keys(tree.Content)) {
		for alias, target := range tree.Content[folder] {
			name := strings.TrimSuffix(alias, ".jpg")
			name = strings.TrimSuffix(name, ".png")
			norm := normalizeName(name)
			if norm == "" || strings.TrimSpace(target) == "" {
				continue
			}

			// Store relative path e.g. Content/folder/target
			rel := fmt.Sprintf("Content/%s/%s", folder, target)
			for _, key := range []string{norm, strings.ReplaceAll(norm, " ", "")} {
				if _, found := index[key]; !found {
					index[key] = rel
				}
			}
		}
	}
	return index
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
		var reader io.Reader = resp.Body
		if limit > 0 {
			reader = io.LimitReader(reader, limit)
		}
		data, err := io.ReadAll(reader)
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

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s
}
