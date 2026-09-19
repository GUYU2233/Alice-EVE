package marketplan

import (
	"context"
	"sync"
	"testing"
	"time"
)

type workerRepoStub struct {
	mu      sync.Mutex
	claims  []Claim
	commits int
	fails   int
}

func (r *workerRepoStub) RequeueWatching(context.Context) (int64, error) { return 0, nil }
func (r *workerRepoStub) Claim(context.Context, time.Time, time.Duration, int) ([]Claim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x := r.claims
	r.claims = nil
	return x, nil
}
func (r *workerRepoStub) Commit(context.Context, Claim, Commit, time.Time) (Job, error) {
	r.mu.Lock()
	r.commits++
	r.mu.Unlock()
	return Job{}, nil
}
func (r *workerRepoStub) Fail(context.Context, Claim, string, time.Time) error {
	r.mu.Lock()
	r.fails++
	r.mu.Unlock()
	return nil
}

type engineStub struct{ fail bool }

func (e engineStub) Step(context.Context, Job) (Commit, error) {
	if e.fail {
		return Commit{}, context.Canceled
	}
	return Commit{State: "optimizing"}, nil
}
func TestWorkerRunsClaimedSlicesAndCommits(t *testing.T) {
	r := &workerRepoStub{claims: []Claim{{Job: Job{ID: "1"}}, {Job: Job{ID: "2"}}}}
	w, e := NewWorker(r, engineStub{}, WorkerConfig{Concurrency: 2, BatchSize: 2, SliceBudget: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	if e = w.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	if r.commits != 2 || r.fails != 0 {
		t.Fatalf("commits=%d fails=%d", r.commits, r.fails)
	}
}
func TestWorkerPersistsFailures(t *testing.T) {
	r := &workerRepoStub{claims: []Claim{{Job: Job{ID: "1"}}}}
	w, _ := NewWorker(r, engineStub{fail: true}, WorkerConfig{})
	_ = w.RunOnce(context.Background())
	if r.fails != 1 {
		t.Fatalf("fails=%d", r.fails)
	}
}
