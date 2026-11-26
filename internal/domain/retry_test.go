package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterFormats(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"seconds", " 60 ", time.Minute},
		{"http date", now.Add(time.Minute).Format(http.TimeFormat), time.Minute},
		{"empty", "", 0},
		{"zero", "0", 0},
		{"negative", "-1", 0},
		{"invalid", "invalid", 0},
		{"fraction", "1.5", 0},
		{"past date", now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"two to the thirty-two seconds", "4294967296", time.Duration(4294967296) * time.Second},
		{"largest whole-second duration", "9223372036", time.Duration(9223372036) * time.Second},
		{"above duration range", "9223372037", time.Duration(math.MaxInt64)},
		{"saturates huge values", "18446744073709551615", time.Duration(math.MaxInt64)},
		{"above uint64 range", "18446744073709551616", time.Duration(math.MaxInt64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRetryAfter(tc.value, now); got != tc.want {
				t.Fatalf("ParseRetryAfter(%q)=%v want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestRetryDelayClassifiesCauses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
		delay time.Duration
	}{
		{"timeout", fmt.Errorf("query: %w", context.DeadlineExceeded), true, 0},
		{"connection", &net.OpError{Op: "dial", Err: errors.New("refused")}, true, 0},
		{"truncated", io.ErrUnexpectedEOF, true, 0},
		{"rate limit", &HTTPError{StatusCode: 429, RetryAfter: time.Minute}, true, time.Minute},
		{"unavailable", &HTTPError{StatusCode: 503}, true, 0},
		{"forbidden", &HTTPError{StatusCode: 403}, false, 0},
		{"missing", &HTTPError{StatusCode: 404}, false, 0},
		{"parser", E(KindUpstream, "invalid document", errors.New("syntax")), false, 0},
		{"conflict", E(KindConflict, "wrong identity", context.DeadlineExceeded), false, 0},
		{"cancel", fmt.Errorf("stopped: %w", context.Canceled), false, 0},
		{"candidates", E(KindUpstream, "images", errors.Join(errors.New("bad image"), &HTTPError{StatusCode: 503, RetryAfter: time.Minute})), true, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delay, retry := RetryDelay(tc.err)
			if delay != tc.delay || retry != tc.retry {
				t.Fatalf("got %v %v", delay, retry)
			}
		})
	}
}
