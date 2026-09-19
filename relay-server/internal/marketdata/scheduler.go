package marketdata

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

const (
	DefaultSchedulerPollInterval = 5 * time.Second
	DefaultSchedulerLease        = 2 * time.Minute
	DefaultSchedulerMaxRegions   = 2
)

type RegionSchedule struct {
	RegionID                                  int64
	Enabled                                   bool
	Tier                                      string
	Priority                                  int
	RefreshInterval, MinRefreshInterval       time.Duration
	MaxStaleness                              time.Duration
	LastRequestedAt, NextRunAt, LastSuccessAt time.Time
	ConsecutiveFailures                       int
	BackoffUntil                              time.Time
	AccessScore, ChangeScore                  float64
}

type RegionStatus struct {
	Schedule RegionSchedule
	Job      *CollectionJob
	Snapshot *Snapshot
}

type SchedulerRepository interface {
	JobRepository
	EnqueueDue(context.Context, time.Time, int) (int64, error)
	MarkScheduleSuccess(context.Context, int64, time.Time) error
	MarkScheduleFailure(context.Context, int64, time.Time) error
	RegionStatus(context.Context, int64) (RegionStatus, error)
}

type RegionCollector interface {
	Collect(context.Context, int64) (Snapshot, error)
}

type SchedulerConfig struct {
	PollInterval time.Duration
	Lease        time.Duration
	MaxRegions   int
	BatchSize    int
	Now          func() time.Time
}

type Scheduler struct {
	repo      SchedulerRepository
	collector RegionCollector
	config    SchedulerConfig
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func NewScheduler(repo SchedulerRepository, collector RegionCollector, config SchedulerConfig) (*Scheduler, error) {
	if repo == nil || collector == nil {
		return nil, errors.New("scheduler repository and collector are required")
	}
	if config.PollInterval == 0 {
		config.PollInterval = DefaultSchedulerPollInterval
	}
	if config.Lease == 0 {
		config.Lease = DefaultSchedulerLease
	}
	if config.MaxRegions == 0 {
		config.MaxRegions = DefaultSchedulerMaxRegions
	}
	if config.BatchSize == 0 {
		config.BatchSize = MaxClaimBatch
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.PollInterval <= 0 || !validLease(config.Lease) || config.MaxRegions < 1 || config.BatchSize < 1 || config.BatchSize > MaxClaimBatch {
		return nil, ErrInvalidJob
	}
	return &Scheduler{repo: repo, collector: collector, config: config, done: make(chan struct{})}, nil
}

func (s *Scheduler) Start(parent context.Context) {
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		s.cancel = cancel
		go s.run(ctx)
	})
}

func (s *Scheduler) Close() {
	s.closeOnce.Do(func() {
		if s.cancel == nil {
			close(s.done)
			return
		}
		s.cancel()
		<-s.done
	})
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	sem := make(chan struct{}, s.config.MaxRegions)
	var wg sync.WaitGroup
	defer wg.Wait()
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	for {
		s.tick(ctx, sem, &wg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup) {
	now := s.config.Now().UTC()
	_, _ = s.repo.RecoverExpired(ctx, now, s.config.BatchSize)
	_, _ = s.repo.EnqueueDue(ctx, now, s.config.BatchSize)
	available := cap(sem) - len(sem)
	if available < 1 {
		return
	}
	jobs, err := s.repo.Claim(ctx, now, s.config.Lease, min(available, s.config.BatchSize))
	if err != nil {
		return
	}
	for _, job := range jobs {
		sem <- struct{}{}
		wg.Add(1)
		go func(job CollectionJob) {
			defer wg.Done()
			defer func() { <-sem }()
			s.process(ctx, job)
		}(job)
	}
}

func (s *Scheduler) process(parent context.Context, job CollectionJob) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		interval := s.config.Lease / 3
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.repo.RenewLease(ctx, job.ID, job.ClaimToken, s.config.Now().UTC(), s.config.Lease); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	_, collectErr := s.collector.Collect(ctx, job.RegionID)
	cancel()
	<-leaseDone
	now := s.config.Now().UTC()
	if collectErr == nil {
		if err := s.repo.CompleteJob(context.WithoutCancel(parent), job.ID, job.ClaimToken, now); err == nil {
			_ = s.repo.MarkScheduleSuccess(context.WithoutCancel(parent), job.RegionID, now)
		}
		return
	}
	code := "collection_failed"
	if errors.Is(collectErr, context.Canceled) {
		code = "collection_canceled"
	}
	if err := s.repo.FailJob(context.WithoutCancel(parent), job.ID, job.ClaimToken, now, code, truncateUTF8(collectErr.Error(), MaxJobFailureDetail)); err == nil {
		_ = s.repo.MarkScheduleFailure(context.WithoutCancel(parent), job.RegionID, now)
	}
}

func tierInterval(tier string) time.Duration {
	switch tier {
	case "A":
		return 12 * time.Minute
	case "B":
		return 45 * time.Minute
	case "C":
		return 2 * time.Hour
	default:
		return 12 * time.Hour
	}
}

func deterministicJitter(regionID int64, at time.Time) float64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%d:%d", regionID, at.UTC().Unix()/86400)
	return (float64(h.Sum32()%4001) / 10000) - 0.20
}

func nextRun(regionID int64, at time.Time, interval time.Duration, failures int) time.Time {
	if failures > 0 {
		return at.Add(failureBackoff(failures))
	}
	return at.Add(time.Duration(float64(interval) * (1 + deterministicJitter(regionID, at))))
}

func failureBackoff(failures int) time.Duration {
	steps := []time.Duration{time.Minute, 3 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour}
	if failures < 1 {
		failures = 1
	}
	if failures > len(steps) {
		failures = len(steps)
	}
	return steps[failures-1]
}
