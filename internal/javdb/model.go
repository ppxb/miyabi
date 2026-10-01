package javdb

import (
	"fmt"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/netx"
)

const (
	appVersion       = "1.9.28"
	appVersionNumber = "10928"
	userAgent        = "Dart/3.4 (dart:io)"
	defaultLanguage  = "zh-TW"
	defaultTimeout   = 20 * time.Second
	defaultRate      = 2.0
	defaultBurst     = 1
)

// Options configures the anonymous JavDB App API client.
type Options struct {
	CachedHost    string
	CachedLatency time.Duration
	DeviceUUID    string
	Proxy         *netx.ProxyManager
	Timeout       time.Duration
}

// RouteStatus describes the currently selected API route.
type RouteStatus struct {
	Host       string
	Latency    time.Duration
	Candidates []RouteCandidate
}

type RouteAvailability string

const (
	RouteUntested    RouteAvailability = "untested"
	RouteAvailable   RouteAvailability = "available"
	RouteUnavailable RouteAvailability = "unavailable"
)

type RouteCandidate struct {
	Host    string
	Latency time.Duration
	Status  RouteAvailability
}

// APIError is returned when JavDB responds with success: 0.
type APIError struct {
	Action  string
	Message string
}

func (e *APIError) Error() string {
	if e.Action == "" && e.Message == "" {
		return "JavDB API error"
	}
	if e.Action == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Action
	}
	return fmt.Sprintf("%s: %s", e.Action, e.Message)
}

func (e *APIError) DomainKind() domain.Kind { return domain.KindUpstream }

func (e *APIError) PublicMessage() string {
	if e.Message == "" {
		return "JavDB 返回了错误"
	}
	return "JavDB 返回了错误：" + e.Message
}

// HTTPError is returned for a non-success HTTP status.
type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("JavDB returned HTTP %d", e.StatusCode)
}

func (e *HTTPError) DomainKind() domain.Kind { return domain.KindUpstream }

func (e *HTTPError) Retryable() bool {
	return (&domain.HTTPError{StatusCode: e.StatusCode}).Retryable()
}
func (e *HTTPError) RetryWait() time.Duration { return e.RetryAfter }

func (e *HTTPError) PublicMessage() string {
	if e.StatusCode == 429 {
		return "JavDB 请求过于频繁，请稍后重试"
	}
	return fmt.Sprintf("JavDB 服务异常（HTTP %d），请稍后重试", e.StatusCode)
}
