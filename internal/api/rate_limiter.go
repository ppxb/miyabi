package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var ErrTooManyLoginAttempts = domain.E(domain.KindRateLimited, "尝试登录失败次数过多，已被封禁 24 小时", nil)

type ipLoginRecord struct {
	failures  int
	firstFail time.Time
	blockedTo time.Time
}

type loginRateLimiter struct {
	mu            sync.Mutex
	records       map[string]*ipLoginRecord
	maxFailures   int
	window        time.Duration
	blockDuration time.Duration
}

func newLoginRateLimiter(maxFailures int, window, blockDuration time.Duration) *loginRateLimiter {
	return &loginRateLimiter{
		records:       make(map[string]*ipLoginRecord),
		maxFailures:   maxFailures,
		window:        window,
		blockDuration: blockDuration,
	}
}

func (l *loginRateLimiter) check(ip string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	rec, exists := l.records[ip]
	if !exists {
		return nil
	}

	now := time.Now()
	if rec.blockedTo.After(now) {
		remaining := rec.blockedTo.Sub(now).Round(time.Minute)
		if remaining < time.Minute {
			remaining = time.Minute
		}
		hours := int(remaining.Hours())
		minutes := int(remaining.Minutes()) % 60
		var timeStr string
		if hours > 0 {
			timeStr = fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
		} else {
			timeStr = fmt.Sprintf("%d 分钟", minutes)
		}
		return domain.E(domain.KindRateLimited, fmt.Sprintf("尝试登录失败次数过多，已被封禁（剩余 %s）", timeStr), ErrTooManyLoginAttempts)
	}

	// If block has expired or window has elapsed, clean up
	if now.Sub(rec.firstFail) > l.window && rec.blockedTo.Before(now) {
		delete(l.records, ip)
	}

	return nil
}

func (l *loginRateLimiter) recordFailure(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rec, exists := l.records[ip]
	if !exists || now.Sub(rec.firstFail) > l.window {
		l.records[ip] = &ipLoginRecord{
			failures:  1,
			firstFail: now,
		}
		return
	}

	rec.failures++
	if rec.failures >= l.maxFailures {
		rec.blockedTo = now.Add(l.blockDuration)
	}
}

func (l *loginRateLimiter) recordSuccess(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, ip)
}
