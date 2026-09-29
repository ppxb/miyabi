package domain

import (
	"errors"
	"fmt"
	"io/fs"
)

// Kind classifies a domain error to determine response status and handling behavior.
type Kind int

const (
	KindUnexpected   Kind = iota // 500 Internal Server Error
	KindInvalid                  // 400 Bad Request
	KindUnauthorized             // 401 Unauthorized
	KindNotFound                 // 404 Not Found
	KindConflict                 // 409 Conflict
	KindBusy                     // 409 Conflict
	KindUpstream                 // 502 Bad Gateway
	KindCanceled                 // 499 Client Closed Request
	KindRateLimited              // 429 Too Many Requests
)

func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindUnauthorized:
		return "unauthorized"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindBusy:
		return "busy"
	case KindUpstream:
		return "upstream"
	case KindCanceled:
		return "canceled"
	case KindRateLimited:
		return "rate_limited"
	default:
		return "unexpected"
	}
}

// Error represents a structured domain error with a user-facing public message
// and an optional underlying diagnostic cause.
//
// Errors are matched by identity: errors.Is(err, sentinel) succeeds only when
// the same *Error value appears in the unwrap chain. Classification by Kind is
// a separate concern; use IsKind or KindOf for that.
type Error struct {
	Kind    Kind
	Message string
	Cause   error
}

// E creates a new domain Error.
func E(kind Kind, message string, cause error) *Error {
	return &Error{
		Kind:    kind,
		Message: message,
		Cause:   cause,
	}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil && e.Message != "" {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Kind.String()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *Error) DomainKind() Kind { return e.Kind }

// PublicMessage returns the user-facing message safe to be exposed via API.
func (e *Error) PublicMessage() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case KindUnexpected:
		return "内部服务错误"
	}
	if e.Message != "" {
		return e.Message
	}
	switch e.Kind {
	case KindInvalid:
		return "请求参数无效"
	case KindUnauthorized:
		return "未授权访问"
	case KindNotFound:
		return "资源不存在"
	case KindConflict:
		return "资源状态冲突"
	case KindBusy:
		return "服务正忙，请稍后重试"
	case KindRateLimited:
		return "请求过于频繁，请稍后重试"
	case KindUpstream:
		return "上游服务异常"
	case KindCanceled:
		return "请求已取消"
	default:
		return "内部服务错误"
	}
}

// IsKind uses the same classification as KindOf. A nil error matches no kind.
func IsKind(err error, kind Kind) bool {
	return err != nil && KindOf(err) == kind
}

// HasKind is the common classification contract for Error and external errors.
type HasKind interface {
	DomainKind() Kind
}

// PublicMessage returns the user-facing text for any error: the first
// PublicMessage() in the unwrap chain, or a default safe message for unexpected errors.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	var public interface{ PublicMessage() string }
	if errors.As(err, &public) {
		if msg := public.PublicMessage(); msg != "" {
			return msg
		}
	}
	switch KindOf(err) {
	case KindUnexpected:
		return "内部服务错误"
	default:
		return err.Error()
	}
}

// KindOf uses the first HasKind in errors.As traversal (outer wrappers take
// precedence; joined errors are visited depth-first, left-to-right). Unclassified
// missing-file errors are NotFound; other errors and nil return KindUnexpected.
func KindOf(err error) Kind {
	if err == nil {
		return KindUnexpected
	}
	var hk HasKind
	if errors.As(err, &hk) {
		return hk.DomainKind()
	}
	if errors.Is(err, fs.ErrNotExist) {
		return KindNotFound
	}
	return KindUnexpected
}
