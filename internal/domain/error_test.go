package domain

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

type classifiedError struct {
	kind  Kind
	cause error
}

func (e classifiedError) Error() string    { return "classified error" }
func (e classifiedError) Unwrap() error    { return e.cause }
func (e classifiedError) DomainKind() Kind { return e.kind }

func TestErrorClassificationUsesOutermostKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want Kind
	}{
		{"external", classifiedError{kind: KindUpstream}, KindUpstream},
		{"wrapped external", fmt.Errorf("request: %w", classifiedError{kind: KindUpstream}), KindUpstream},
		{"external overrides domain cause", classifiedError{KindInvalid, E(KindUnauthorized, "", nil)}, KindInvalid},
		{"domain overrides external cause", E(KindConflict, "", classifiedError{kind: KindUpstream}), KindConflict},
		{"joined errors use first classifier", errors.Join(classifiedError{kind: KindCanceled}, E(KindUpstream, "", nil)), KindCanceled},
		{"file missing", fmt.Errorf("open: %w", fs.ErrNotExist), KindNotFound},
		{"explicit kind overrides file missing", E(KindInvalid, "", fs.ErrNotExist), KindInvalid},
		{"unknown", errors.New("database failed"), KindUnexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := KindOf(tc.err); got != tc.want {
				t.Fatalf("kind=%v want=%v", got, tc.want)
			}
			for kind := KindUnexpected; kind <= KindRateLimited; kind++ {
				if got := IsKind(tc.err, kind); got != (kind == tc.want) {
					t.Fatalf("IsKind(%v)=%v want kind %v", kind, got, tc.want)
				}
			}
		})
	}
	if KindOf(nil) != KindUnexpected || IsKind(nil, KindUnexpected) {
		t.Fatal("nil must not match an error kind")
	}
	if got := PublicMessage(E(KindUnexpected, "SQL details", errors.New("private path"))); got != "内部服务错误" {
		t.Fatalf("internal details exposed: %s", got)
	}
	if got := PublicMessage(E(KindRateLimited, "", nil)); got != "请求过于频繁，请稍后重试" {
		t.Fatalf("rate-limit message: %s", got)
	}
}

func TestDomainError(t *testing.T) {
	t.Run("basic error formatting and unwrapping", func(t *testing.T) {
		cause := errors.New("underlying network timeout")
		err := E(KindUpstream, "上游服务无响应", cause)

		if err.Kind != KindUpstream {
			t.Errorf("Kind = %v, want %v", err.Kind, KindUpstream)
		}
		if got := err.Error(); got != "上游服务无响应: underlying network timeout" {
			t.Errorf("Error() = %q, want %q", got, "上游服务无响应: underlying network timeout")
		}
		if got := err.PublicMessage(); got != "上游服务无响应" {
			t.Errorf("PublicMessage() = %q, want %q", got, "上游服务无响应")
		}
		if !errors.Is(err, cause) {
			t.Errorf("errors.Is(err, cause) = false, want true")
		}
		if errors.Unwrap(err) != cause {
			t.Errorf("Unwrap() = %v, want %v", errors.Unwrap(err), cause)
		}
	})

	t.Run("default public messages", func(t *testing.T) {
		err := E(KindNotFound, "", nil)
		if got := err.PublicMessage(); got != "资源不存在" {
			t.Errorf("PublicMessage() = %q, want %q", got, "资源不存在")
		}
	})

	t.Run("public message masking for unexpected errors", func(t *testing.T) {
		if got := PublicMessage(nil); got != "" {
			t.Errorf("PublicMessage(nil) = %q, want empty", got)
		}
		if got := PublicMessage(errors.New("UNIQUE constraint failed: movies.code")); got != "内部服务错误" {
			t.Errorf("PublicMessage(unknown) = %q, want %q", got, "内部服务错误")
		}
		if got := PublicMessage(E(KindUnexpected, "", errors.New("db crash"))); got != "内部服务错误" {
			t.Errorf("PublicMessage(KindUnexpected) = %q, want %q", got, "内部服务错误")
		}
		if got := PublicMessage(E(KindUnexpected, "", errors.New("panic"))); got != "内部服务错误" {
			t.Errorf("PublicMessage(KindUnexpected) = %q, want %q", got, "内部服务错误")
		}
		if got := PublicMessage(E(KindNotFound, "", nil)); got != "资源不存在" {
			t.Errorf("PublicMessage(KindNotFound) = %q, want %q", got, "资源不存在")
		}
	})

	t.Run("Is and IsKind", func(t *testing.T) {
		sentinel := E(KindInvalid, "参数无效", nil)
		wrapped := fmt.Errorf("wrap: %w", sentinel)

		if !IsKind(wrapped, KindInvalid) {
			t.Errorf("IsKind(wrapped, KindInvalid) = false, want true")
		}
		if IsKind(wrapped, KindNotFound) {
			t.Errorf("IsKind(wrapped, KindNotFound) = true, want false")
		}
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is(wrapped, sentinel) = false, want true")
		}
		if KindOf(wrapped) != KindInvalid {
			t.Errorf("KindOf(wrapped) = %v, want %v", KindOf(wrapped), KindInvalid)
		}
	})

	t.Run("sentinels match by identity only", func(t *testing.T) {
		first := E(KindInvalid, "参数无效", nil)
		second := E(KindInvalid, "参数无效", nil)
		if errors.Is(first, second) {
			t.Errorf("errors.Is matched two distinct sentinels with equal fields")
		}
		if !errors.Is(fmt.Errorf("wrap: %w", first), first) {
			t.Errorf("errors.Is lost the sentinel through wrapping")
		}
	})
}
