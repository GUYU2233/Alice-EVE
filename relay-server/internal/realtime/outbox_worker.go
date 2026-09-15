package realtime

import (
	"context"
	"sync"
	"time"
)

// OutboxWorker drives an OutboxDispatcher in the background. PostgreSQL
// notifications are treated as a best-effort wakeup; the poll ticker remains
// authoritative so missed notifications and rows written by other processes
// are eventually processed.
type OutboxWorker struct {
	dispatcher   *OutboxDispatcher
	wakeups      <-chan Wakeup
	pollInterval time.Duration
	batchSize    int

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	close  sync.Once
}

// NewOutboxWorker constructs an idle worker. Call Start to launch its loop.
// A nil wakeups channel is valid and simply disables notification-triggered
// dispatches while retaining periodic polling.
func NewOutboxWorker(dispatcher *OutboxDispatcher, wakeups <-chan Wakeup, pollInterval time.Duration) *OutboxWorker {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &OutboxWorker{
		dispatcher: dispatcher, wakeups: wakeups,
		pollInterval: pollInterval, batchSize: 100,
		done: make(chan struct{}),
	}
}

// Start starts the worker. It is safe to call more than once; only the first
// call launches a goroutine.
func (w *OutboxWorker) Start(parent context.Context) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	w.ctx, w.cancel = context.WithCancel(parent)
	ctx := w.ctx
	w.mu.Unlock()
	go w.run(ctx)
}

func (w *OutboxWorker) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	dispatch := func() {
		if w.dispatcher == nil {
			return
		}
		// Drain full batches without waiting for a ticker, but stop after a
		// partial batch to avoid spinning when the outbox is empty.
		for {
			n, err := w.dispatcher.Dispatch(ctx, w.batchSize, time.Now().UTC())
			if err != nil || n < w.batchSize {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}

	// Process rows already present before waiting for a notification.
	dispatch()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-w.wakeups:
			if !ok {
				// A closed notifier channel must not turn the select into a hot
				// loop. Polling continues as the durable fallback.
				w.wakeups = nil
				continue
			}
			dispatch()
		case <-ticker.C:
			dispatch()
		}
	}
}

// Close stops the worker and waits for its goroutine. It is safe to call
// repeatedly, including when Start was never called.
func (w *OutboxWorker) Close() error {
	if w == nil {
		return nil
	}
	w.close.Do(func() {
		w.mu.Lock()
		cancel := w.cancel
		w.mu.Unlock()
		if cancel != nil {
			cancel()
			<-w.done
		}
	})
	return nil
}
