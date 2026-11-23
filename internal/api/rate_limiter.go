package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var ErrTooManyLoginAttempts = domain.E(domain.KindRateLimited, "登录尝试过于频繁，请稍后再试", nil)

const (
	maxLoginRecords          = 4096
	loginRecordSweepInterval = time.Minute
)

type loginAttemptResult uint8

const (
	loginNotVerified loginAttemptResult = iota
	loginFailed
	loginSucceeded
)

type ipLoginRecord struct {
	failures  int
	pending   int
	firstFail time.Time
	blockedTo time.Time
}

type loginRateLimiter struct {
	mu            sync.Mutex
	records       map[string]*ipLoginRecord
	maxFailures   int
	window        time.Duration
	blockDuration time.Duration
	nextSweep     time.Time
}

func newLoginRateLimiter(maxFailures int, window, blockDuration time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		records:       make(map[string]*ipLoginRecord),
		maxFailures:   maxFailures,
		window:        window,
		blockDuration: blockDuration,
	}
}

// begin reserves an attempt before parsing or password verification. Pending
// attempts count against the limit so parallel requests cannot all pass it.
func (l *loginRateLimiter) begin(ip string) (*ipLoginRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if !now.Before(l.nextSweep) {
		for key, rec := range l.records {
			if rec.pending == 0 && !now.Before(rec.expiresAt(l.window)) {
				delete(l.records, key)
			}
		}
		l.nextSweep = now.Add(min(loginRecordSweepInterval, l.window, l.blockDuration))
	}

	rec := l.records[ip]
	if rec == nil {
		if len(l.records) >= maxLoginRecords && !l.evictSoonestExpiring() {
			return nil, ErrTooManyLoginAttempts
		}
		rec = &ipLoginRecord{}
		l.records[ip] = rec
	}
	if rec.blockedTo.After(now) {
		remaining := int((rec.blockedTo.Sub(now) + time.Minute - 1) / time.Minute)
		hours, minutes := remaining/60, remaining%60
		var timeStr string
		if hours > 0 {
			timeStr = fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
		} else {
			timeStr = fmt.Sprintf("%d 分钟", minutes)
		}
		return nil, domain.E(domain.KindRateLimited, fmt.Sprintf("尝试登录失败次数过多，已被封禁（剩余 %s）", timeStr), ErrTooManyLoginAttempts)
	}

	if !now.Before(rec.expiresAt(l.window)) {
		rec.failures, rec.firstFail, rec.blockedTo = 0, time.Time{}, time.Time{}
	}
	if rec.failures+rec.pending >= l.maxFailures {
		return nil, ErrTooManyLoginAttempts
	}
	rec.pending++
	return rec, nil
}

// finish is called once for every admitted attempt. Keep the record while
// other attempts are pending, including when one of them succeeds.
func (l *loginRateLimiter) finish(ip string, rec *ipLoginRecord, result loginAttemptResult) {
	l.mu.Lock()
	defer l.mu.Unlock()

	rec.pending--
	switch result {
	case loginSucceeded:
		rec.failures, rec.firstFail, rec.blockedTo = 0, time.Time{}, time.Time{}
	case loginFailed:
		now := time.Now()
		if rec.firstFail.IsZero() || !now.Before(rec.firstFail.Add(l.window)) {
			rec.failures, rec.firstFail = 0, now
		}
		rec.failures++
		if rec.failures >= l.maxFailures {
			rec.blockedTo = now.Add(l.blockDuration)
		}
	}
	if rec.pending == 0 && rec.failures == 0 {
		delete(l.records, ip)
	}
}

// evictSoonestExpiring makes room in a full table by dropping the idle record
// that would expire first, so short failure windows go before long bans.
// Records with pending attempts are never evicted because finish still holds
// them; it reports false when every record is pending.
func (l *loginRateLimiter) evictSoonestExpiring() bool {
	var victim string
	var soonest time.Time
	found := false
	for key, rec := range l.records {
		if rec.pending > 0 {
			continue
		}
		if at := rec.expiresAt(l.window); !found || at.Before(soonest) {
			victim, soonest, found = key, at, true
		}
	}
	if found {
		delete(l.records, victim)
	}
	return found
}

func (rec *ipLoginRecord) expiresAt(window time.Duration) time.Time {
	if !rec.blockedTo.IsZero() {
		return rec.blockedTo
	}
	return rec.firstFail.Add(window)
}
