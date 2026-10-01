package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

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
