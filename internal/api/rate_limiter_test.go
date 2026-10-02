package api

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func completeLoginAttempt(t *testing.T, limiter *loginRateLimiter, ip string, result loginAttemptResult) {
	t.Helper()
	attempt, err := limiter.begin(ip)
	if err != nil {
		t.Fatalf("admit %s: %v", ip, err)
	}
	limiter.finish(ip, attempt, result)
}

func TestLoginRateLimiterBlocksAfterMaxFailures(t *testing.T) {
	for _, limit := range []int{1, 3} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				limiter := newLoginRateLimiter(limit, 5*time.Minute, 10*time.Minute)
				for range limit {
					completeLoginAttempt(t, limiter, "blocked", loginFailed)
				}
				if _, err := limiter.begin("blocked"); !errors.Is(err, ErrTooManyLoginAttempts) || !strings.Contains(err.Error(), "10 分钟") || strings.Contains(err.Error(), "24 小时") {
					t.Fatalf("unexpected block duration or cause: %v", err)
				}
				completeLoginAttempt(t, limiter, "other", loginSucceeded)
				time.Sleep(5 * time.Minute)
				if _, err := limiter.begin("blocked"); !errors.Is(err, ErrTooManyLoginAttempts) {
					t.Fatalf("failure window expiry released an active block: %v", err)
				}
				time.Sleep(5 * time.Minute)
				completeLoginAttempt(t, limiter, "blocked", loginSucceeded)
			})
		})
	}
}

func TestLoginRateLimiterResetsExpiredWindowsAndShortBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := newLoginRateLimiter(2, 10*time.Minute, time.Minute)
		completeLoginAttempt(t, limiter, "ip", loginFailed)
		time.Sleep(10 * time.Minute)
		completeLoginAttempt(t, limiter, "ip", loginFailed)
		completeLoginAttempt(t, limiter, "ip", loginFailed)
		if _, err := limiter.begin("ip"); !errors.Is(err, ErrTooManyLoginAttempts) {
			t.Fatalf("new failure window did not block: %v", err)
		}
		time.Sleep(time.Minute)
		completeLoginAttempt(t, limiter, "ip", loginSucceeded)
		if len(limiter.records) != 0 {
			t.Fatal("success retained the failure history")
		}
	})
}

func TestLoginRateLimiterSuccessResetsFailuresAndPreservesPendingReservations(t *testing.T) {
	limiter := newLoginRateLimiter(3, 5*time.Minute, time.Hour)
	completeLoginAttempt(t, limiter, "ip", loginFailed)
	completeLoginAttempt(t, limiter, "ip", loginFailed)
	completeLoginAttempt(t, limiter, "ip", loginSucceeded)
	if len(limiter.records) != 0 {
		t.Fatal("success did not clear previous failures")
	}
	var pending []*ipLoginRecord
	for range 3 {
		attempt, err := limiter.begin("ip")
		if err != nil {
			t.Fatal(err)
		}
		pending = append(pending, attempt)
	}
	if _, err := limiter.begin("ip"); !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("parallel attempts exceeded limit: %v", err)
	}
	limiter.finish("ip", pending[0], loginSucceeded)
	last, err := limiter.begin("ip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limiter.begin("ip"); !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("success erased other pending reservations: %v", err)
	}
	limiter.finish("ip", pending[1], loginFailed)
	limiter.finish("ip", pending[2], loginFailed)
	limiter.finish("ip", last, loginFailed)
	if _, err := limiter.begin("ip"); !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("late failures were lost after success: %v", err)
	}
}

func TestLoginRateLimiterCapacityAndExpiredRecordReclamation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := newLoginRateLimiter(2, 5*time.Minute, time.Hour)
		completeLoginAttempt(t, limiter, "banned", loginFailed)
		completeLoginAttempt(t, limiter, "banned", loginFailed)
		pending, err := limiter.begin("pending")
		if err != nil {
			t.Fatal(err)
		}
		for i := range maxLoginRecords - 2 {
			completeLoginAttempt(t, limiter, fmt.Sprint(i), loginFailed)
		}
		for range 3 {
			if _, err := limiter.begin("new"); !errors.Is(err, ErrTooManyLoginAttempts) {
				t.Fatalf("full limiter admitted an untracked IP: %v", err)
			}
		}
		if len(limiter.records) != maxLoginRecords {
			t.Fatalf("record count=%d", len(limiter.records))
		}
		if _, err := limiter.begin("banned"); !errors.Is(err, ErrTooManyLoginAttempts) {
			t.Fatalf("capacity pressure evicted an active ban: %v", err)
		}
		// Known IPs can still authenticate while the table is full.
		completeLoginAttempt(t, limiter, "0", loginSucceeded)
		completeLoginAttempt(t, limiter, "new", loginNotVerified)
		time.Sleep(5 * time.Minute)
		completeLoginAttempt(t, limiter, "another", loginNotVerified)
		if len(limiter.records) != 2 || limiter.records["pending"] != pending || limiter.records["banned"] == nil {
			t.Fatal("sweep retained expired IPs or removed active records")
		}
		limiter.finish("pending", pending, loginFailed)
		time.Sleep(55 * time.Minute)
		completeLoginAttempt(t, limiter, "last", loginNotVerified)
		if len(limiter.records) != 0 {
			t.Fatal("expired bans and failure records were not reclaimed")
		}
	})
}
