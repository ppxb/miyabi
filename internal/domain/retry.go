package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError preserves status and backoff without classifying parser errors as
// transient failures merely because they came from an upstream service.
type HTTPError struct {
	Source     string
	StatusCode int
	RetryAfter time.Duration
}

// RetryError carries a source-wide cooldown while preserving the original cause.
type RetryError struct {
	Cause error
	After time.Duration
}

func (e *RetryError) Error() string            { return e.Cause.Error() }
func (e *RetryError) Unwrap() error            { return e.Cause }
func (e *RetryError) Retryable() bool          { return true }
func (e *RetryError) RetryWait() time.Duration { return e.After }

func (e *HTTPError) Error() string    { return fmt.Sprintf("%s returned HTTP %d", e.Source, e.StatusCode) }
func (e *HTTPError) DomainKind() Kind { return KindUpstream }
func (e *HTTPError) Retryable() bool {
	return e.StatusCode == 408 || e.StatusCode == 429 || e.StatusCode >= 500 && e.StatusCode < 600
}
func (e *HTTPError) RetryWait() time.Duration { return e.RetryAfter }

func ParseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if until, err := http.ParseTime(value); err == nil && until.After(now) {
		return until.Sub(now)
	}
	return 0
}

// RetryDelay only admits known transient failures. Permanent domain errors
// stop traversal; joined candidate failures may recover if any candidate can.
func RetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	if kind, ok := err.(HasKind); ok {
		switch kind.DomainKind() {
		case KindInvalid, KindUnauthorized, KindNotFound, KindConflict, KindCanceled:
			return 0, false
		case KindRateLimited:
			return 0, true
		}
	}
	if hint, ok := err.(interface {
		Retryable() bool
		RetryWait() time.Duration
	}); ok {
		return hint.RetryWait(), hint.Retryable()
	}
	if err == context.DeadlineExceeded || err == io.EOF || err == io.ErrUnexpectedEOF {
		return 0, true
	}
	if network, ok := err.(net.Error); ok {
		if errors.Is(err, context.Canceled) {
			return 0, false
		}
		if dns, ok := network.(*net.DNSError); ok && dns.IsNotFound {
			return 0, false
		}
		if _, dial := network.(*net.OpError); dial {
			var dns *net.DNSError
			return 0, !errors.As(err, &dns) || !dns.IsNotFound
		}
		if network.Timeout() || network.Temporary() {
			return 0, true
		}
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var delay time.Duration
		var retry bool
		for _, cause := range joined.Unwrap() {
			wait, transient := RetryDelay(cause)
			if transient {
				delay, retry = max(delay, wait), true
			}
		}
		return delay, retry
	}
	return RetryDelay(errors.Unwrap(err))
}
