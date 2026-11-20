package gfriends

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// The production client now keeps a single allImages index. These helpers keep
// older tests that seeded a first-path-only index working, converting it to the
// unified shape.

// buildIndex keeps the first image per actor name, in deterministic folder order.
func buildIndex(tree fileTree) map[string]string {
	index := make(map[string]string)
	for _, folder := range slices.Sorted(maps.Keys(tree.Content)) {
		for _, alias := range slices.Sorted(maps.Keys(tree.Content[folder])) {
			target := tree.Content[folder][alias]
			name := strings.TrimSuffix(strings.TrimSuffix(alias, ".jpg"), ".png")
			norm := normalizeName(name)
			if norm == "" || strings.TrimSpace(target) == "" {
				continue
			}
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

// installIndex seeds the unified index from a first-path-only map.
func (c *Client) installIndex(index map[string]string, at time.Time) {
	all := make(map[string][]string, len(index))
	for key, rel := range index {
		all[key] = []string{rel}
	}
	c.mu.Lock()
	c.allImages = all
	c.loadedAt = at
	c.retryAt = time.Time{}
	c.refreshErr = nil
	c.mu.Unlock()
}

// readCachedIndex exposes the old first-path view over the unified disk cache.
func readCachedIndex(path string) (map[string]string, time.Time, error) {
	all, updatedAt, err := readCachedAll(path)
	if err != nil {
		return nil, updatedAt, err
	}
	index := make(map[string]string, len(all))
	for key, rels := range all {
		if len(rels) > 0 {
			index[key] = rels[0]
		}
	}
	return index, updatedAt, nil
}
