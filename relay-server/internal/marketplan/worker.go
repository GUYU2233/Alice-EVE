package marketplan

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrLeaseLost = errors.New("market plan lease lost")

type Claim struct {
	Job        Job
	Token      string
	LeaseUntil time.Time
}
type Commit struct {
	State    string
	Progress JSONRaw
	Results  []PendingResult
	Frontier JSONRaw
}
type JSONRaw []byte
type PendingResult struct {
	StableKey string
	Score     float64
	Payload   []byte
}
type WorkerRepository interface {
	RequeueWatching(context.Context) (int64, error)
	Claim(context.Context, time.Time, time.Duration, int) ([]Claim, error)
	Commit(context.Context, Claim, Commit, time.Time) (Job, error)
	Fail(context.Context, Claim, string, time.Time) error
}
type Engine interface {
	Step(context.Context, Job) (Commit, error)
}
type WorkerConfig struct {
	PollInterval, Lease, SliceBudget time.Duration
	BatchSize, Concurrency           int
}
type Worker struct {
	repo   WorkerRepository
	engine Engine
	cfg    WorkerConfig
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewWorker(r WorkerRepository, e Engine, c WorkerConfig) (*Worker, error) {
	if r == nil || e == nil {
		return nil, errors.New("worker dependencies required")
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	if c.SliceBudget <= 0 {
		c.SliceBudget = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 4
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 2
	}
	return &Worker{repo: r, engine: e, cfg: c}, nil
}
func (w *Worker) Start(parent context.Context) {
	if w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.cfg.PollInterval)
		defer ticker.Stop()
		for {
			_ = w.RunOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
func (w *Worker) Close() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}
func (w *Worker) RunOnce(ctx context.Context) error {
	if _, e := w.repo.RequeueWatching(ctx); e != nil {
		return e
	}
	claims, e := w.repo.Claim(ctx, time.Now().UTC(), w.cfg.Lease, w.cfg.BatchSize)
	if e != nil {
		return e
	}
	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	for _, claim := range claims {
		claim := claim
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			stepCtx, cancel := context.WithTimeout(ctx, w.cfg.SliceBudget)
			defer cancel()
			commit, err := w.engine.Step(stepCtx, claim.Job)
			now := time.Now().UTC()
			if err != nil {
				_ = w.repo.Fail(ctx, claim, err.Error(), now)
				return
			}
			_, _ = w.repo.Commit(ctx, claim, commit, now)
		}()
	}
	wg.Wait()
	return nil
}
