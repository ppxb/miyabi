package gfriends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	DefaultFastlyURL = "https://fastly.jsdelivr.net/gh/gfriends/gfriends@master"
	DefaultRawURL    = "https://raw.githubusercontent.com/gfriends/gfriends/master"
	CacheExpiration  = 7 * 24 * time.Hour
)

// mirrors serve the same repository; the CDN is tried before GitHub.
var mirrors = []string{DefaultFastlyURL, DefaultRawURL}

type FileTree struct {
	Content map[string]map[string]string `json:"Content"`
}

type Client struct {
	dataDir    string
	httpClient *http.Client
	mu         sync.RWMutex
	index      map[string]string // normalized name -> relative path e.g. "Content/9-Javrave/xxx.jpg?t=..."
	loadedAt   time.Time
}

func New(dataDir string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
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

// EnsureIndex loads the filetree index from disk or downloads it if expired/missing.
func (c *Client) EnsureIndex(ctx context.Context) error {
	c.mu.RLock()
	hasIndex := len(c.index) > 0 && time.Since(c.loadedAt) < CacheExpiration
	c.mu.RUnlock()

	if hasIndex {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.index) > 0 && time.Since(c.loadedAt) < CacheExpiration {
		return nil
	}

	cacheFile := filepath.Join(c.dataDir, "gfriends_tree.json")
	if info, err := os.Stat(cacheFile); err == nil && time.Since(info.ModTime()) < CacheExpiration {
		if err := c.loadFromFile(cacheFile); err == nil {
			return nil
		}
	}

	// Download from remote
	var tree FileTree
	var downloadErr error
	for _, mirror := range mirrors {
		u := mirror + "/Filetree.json"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			downloadErr = err
			continue
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			downloadErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			downloadErr = fmt.Errorf("status code: %d", resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			downloadErr = err
			continue
		}

		if err := json.Unmarshal(body, &tree); err != nil {
			downloadErr = err
			continue
		}

		// Save to cache file
		if c.dataDir != "" {
			_ = os.MkdirAll(c.dataDir, 0755)
			_ = os.WriteFile(cacheFile, body, 0644)
		}

		c.buildIndexLocked(tree)
		c.loadedAt = time.Now()
		return nil
	}

	// If download failed but we have an old cached file, fallback to it
	if _, err := os.Stat(cacheFile); err == nil {
		if err := c.loadFromFile(cacheFile); err == nil {
			return nil
		}
	}

	return fmt.Errorf("download gfriends filetree: %w", downloadErr)
}

func (c *Client) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var tree FileTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return err
	}
	c.buildIndexLocked(tree)
	c.loadedAt = time.Now()
	return nil
}

// buildIndexLocked indexes folders in name order and keeps the first image per
// actor, so the chosen avatar does not depend on map iteration order.
func (c *Client) buildIndexLocked(tree FileTree) {
	c.index = make(map[string]string)
	for _, folder := range slices.Sorted(maps.Keys(tree.Content)) {
		for alias, target := range tree.Content[folder] {
			name := strings.TrimSuffix(alias, ".jpg")
			name = strings.TrimSuffix(name, ".png")
			norm := normalizeName(name)
			if norm == "" {
				continue
			}

			// Store relative path e.g. Content/folder/target
			rel := fmt.Sprintf("Content/%s/%s", folder, target)
			for _, key := range []string{norm, strings.ReplaceAll(norm, " ", "")} {
				if _, found := c.index[key]; !found {
					c.index[key] = rel
				}
			}
		}
	}
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

	var lastErr error
	for _, mirror := range mirrors {
		u := mirror + "/" + escapedPath
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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

		data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}

	return nil, errors.Join(errors.New("fetch avatar failed"), lastErr)
}

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s
}
