package esi

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	DefaultLimiterMaxWait = 2 * time.Minute
	degradedDelay         = 250 * time.Millisecond
)

var ErrLimiterWaitExceeded = errors.New("ESI error-limit wait exceeds maximum")

// Limiter coordinates the ESI error budget across all concurrent users of a
// Client. It does not start background goroutines; waiters use their caller's
// context and timer.
type Limiter struct {
	mu       sync.Mutex
	now      func() time.Time
	maxWait  time.Duration
	remain   int
	until    time.Time
	nextSlow time.Time
	known    bool
}

func NewLimiter(maxWait time.Duration) *Limiter {
	if maxWait <= 0 {
		maxWait = DefaultLimiterMaxWait
	}
	return &Limiter{now: time.Now, maxWait: maxWait}
}

// Wait blocks until a request may start. Above 50 requests remain there is no
// throttling; from 20 through 50 starts are gently spaced; below 20 new work is
// held until the server's reset. Retry-After also sets the same shared hold.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	now := l.now()
	wait := time.Duration(0)
	if l.until.After(now) {
		wait = l.until.Sub(now)
	} else if l.known && l.remain >= 20 && l.remain <= 50 {
		start := now
		if l.nextSlow.After(start) {
			start = l.nextSlow
		}
		wait = start.Sub(now)
		l.nextSlow = start.Add(degradedDelay)
	}
	maxWait := l.maxWait
	l.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	if wait > maxWait {
		return ErrLimiterWaitExceeded
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Observe updates the process-wide view after any ESI response.
func (l *Limiter) Observe(meta Response) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if meta.ErrorLimitRemainSet {
		l.remain = meta.ErrorLimitRemain
		l.known = true
	}
	if meta.ErrorLimitResetSet && meta.ErrorLimitRemainSet && meta.ErrorLimitRemain < 20 {
		until := now.Add(meta.ErrorLimitReset)
		if until.After(l.until) {
			l.until = until
		}
	}
	if (meta.StatusCode == 420 || meta.StatusCode == 429) && meta.RetryAfterSet {
		until := meta.RetryAt
		if until.IsZero() {
			until = now.Add(meta.RetryAfter)
		}
		if until.After(l.until) {
			l.until = until
		}
	}
	if !l.until.After(now) {
		l.until = time.Time{}
	}
}
