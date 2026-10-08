package strm

import (
	"net/url"
	"strconv"
	"time"
)

const (
	streamCacheLimit   = 256
	streamCacheTTL     = time.Minute
	streamExpiryMargin = 30 * time.Second
)

type cachedURL struct {
	address string
	expires time.Time
}

func (relay *Relay) cached(key string, now time.Time) string {
	relay.cacheMu.Lock()
	defer relay.cacheMu.Unlock()
	for key, entry := range relay.urls {
		if !now.Before(entry.expires) {
			delete(relay.urls, key)
		}
	}
	return relay.urls[key].address
}

func (relay *Relay) remember(key, address string, now time.Time) {
	expires := streamCacheExpiry(address, now)
	if !now.Before(expires) {
		return
	}
	relay.cacheMu.Lock()
	defer relay.cacheMu.Unlock()
	if relay.urls == nil {
		relay.urls = make(map[string]cachedURL)
	}
	for key, entry := range relay.urls {
		if !now.Before(entry.expires) {
			delete(relay.urls, key)
		}
	}
	if len(relay.urls) >= streamCacheLimit {
		var oldest string
		var deadline time.Time
		for key, entry := range relay.urls {
			if deadline.IsZero() || entry.expires.Before(deadline) {
				oldest, deadline = key, entry.expires
			}
		}
		delete(relay.urls, oldest)
	}
	relay.urls[key] = cachedURL{address: address, expires: expires}
}

// 115 download links carry a Unix expiry in t; explicit expires timestamps are
// also accepted. Unknown or malformed expiry formats are never persisted in the
// cache. Keep the original URL intact and stop reusing it before its signature ends.
func streamCacheExpiry(address string, now time.Time) time.Time {
	parsed, err := url.Parse(address)
	if err != nil {
		return time.Time{}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return time.Time{}
	}
	expires := now.Add(streamCacheTTL)
	known := false
	for _, name := range []string{"t", "expires"} {
		for _, value := range query[name] {
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil || seconds <= 0 {
				return time.Time{}
			}
			deadline := time.Unix(seconds, 0).Add(-streamExpiryMargin)
			if deadline.Before(expires) {
				expires = deadline
			}
			known = true
		}
	}
	if !known {
		return time.Time{}
	}
	return expires
}
