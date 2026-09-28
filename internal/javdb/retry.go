package javdb

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxRateLimitRetries = 3
	maxRateLimitWait    = 2 * time.Minute
)

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds > math.MaxInt64/uint64(time.Second) {
			return time.Duration(math.MaxInt64)
		}
		return time.Duration(seconds) * time.Second
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until.Sub(now)
	}
	return 0
}

// All API callers share the cooldown, including library and subscription workers.
// A late response must never shorten an already announced Retry-After interval.
func (c *Client) deferRequests(delay time.Duration) {
	until := time.Now().Add(delay)
	c.cooldownMu.Lock()
	if until.After(c.cooldownUntil) {
		c.cooldownUntil = until
	}
	c.cooldownMu.Unlock()
}

func (c *Client) cooldown() time.Duration {
	c.cooldownMu.Lock()
	defer c.cooldownMu.Unlock()
	return time.Until(c.cooldownUntil)
}

func (c *Client) waitRequest(ctx context.Context, deadline time.Time) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if delay := c.cooldown(); delay > 0 {
			// Do not truncate Retry-After and hit the server early. Long cooldowns
			// survive this call; the caller can retry the failed item later.
			if delay > time.Until(deadline) {
				return &HTTPError{StatusCode: 429, RetryAfter: delay}
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		// Another in-flight request may have received 429 while this one waited
		// for its rate token. Recheck, and pace callers again after the cooldown.
		if c.cooldown() <= 0 {
			return nil
		}
	}
}
