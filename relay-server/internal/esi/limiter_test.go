package esi

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLimiterThresholdsAndReset(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewLimiter(time.Minute)
	l.now = func() time.Time { return now }

	l.Observe(Response{ErrorLimitRemain: 51, ErrorLimitRemainSet: true})
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	l.Observe(Response{ErrorLimitRemain: 19, ErrorLimitRemainSet: true, ErrorLimitReset: 10 * time.Second, ErrorLimitResetSet: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error=%v", err)
	}
	now = now.Add(10 * time.Second)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("after reset: %v", err)
	}
}

func TestLimiterRetryAfterAndHardWaitCap(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewLimiter(5 * time.Second)
	l.now = func() time.Time { return now }
	l.Observe(Response{StatusCode: 429, RetryAfter: 6 * time.Second, RetryAt: now.Add(6 * time.Second), RetryAfterSet: true})
	if err := l.Wait(context.Background()); !errors.Is(err, ErrLimiterWaitExceeded) {
		t.Fatalf("wait error=%v", err)
	}
}

func TestLimiterConcurrentObserveAndCanceledWait(t *testing.T) {
	l := NewLimiter(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(remain int) {
			defer wg.Done()
			l.Observe(Response{ErrorLimitRemain: remain, ErrorLimitRemainSet: true, ErrorLimitReset: time.Second, ErrorLimitResetSet: true})
			_ = l.Wait(ctx)
		}(i)
	}
	wg.Wait()
}
