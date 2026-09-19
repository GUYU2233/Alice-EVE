package marketdata

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relay-server/internal/esi"
)

type fakeGateway struct {
	pages         int
	failAt        int
	changedAt     int
	block         <-chan struct{}
	calls         atomic.Int32
	active        atomic.Int32
	maxActive     atomic.Int32
	cancellations atomic.Int32
}

func (g *fakeGateway) RegionOrdersPage(ctx context.Context, _ int64, page int) ([]esi.MarketOrder, esi.Response, error) {
	g.calls.Add(1)
	active := g.active.Add(1)
	defer g.active.Add(-1)
	for {
		old := g.maxActive.Load()
		if active <= old || g.maxActive.CompareAndSwap(old, active) {
			break
		}
	}
	if page == g.failAt {
		return nil, esi.Response{}, errors.New("boom")
	}
	if page > 1 && g.block != nil {
		select {
		case <-g.block:
		case <-ctx.Done():
			g.cancellations.Add(1)
			return nil, esi.Response{}, ctx.Err()
		}
	}
	pages := g.pages
	if pages == 0 {
		pages = 2
	}
	if page == g.changedAt {
		pages++
	}
	return []esi.MarketOrder{{OrderID: int64(page), TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5, VolumeTotal: 10, VolumeRemain: 9, MinVolume: 1, Range: "region", Duration: 90, Issued: time.Unix(1, 0)}}, esi.Response{Pages: pages, ExpiresAt: time.Unix(100, 0)}, nil
}

type fakeRepo struct {
	mu           sync.Mutex
	pages        map[int]bool
	failed       int
	published    bool
	appendFailAt int
}

func (r *fakeRepo) BeginBatch(context.Context, int64, time.Time) (Batch, error) {
	return Batch{ID: "batch"}, nil
}
func (r *fakeRepo) AppendPage(_ context.Context, _ string, page, _ int, _ []Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if page == r.appendFailAt {
		return errors.New("append boom")
	}
	if r.pages == nil {
		r.pages = make(map[int]bool)
	}
	r.pages[page] = true
	return nil
}
func (r *fakeRepo) Publish(context.Context, string, time.Time, time.Time, string) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = true
	return Snapshot{RegionID: 1}, nil
}
func (r *fakeRepo) Fail(context.Context, string, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed++
	return nil
}
func (*fakeRepo) Snapshot(context.Context, int64) (Snapshot, error) { return Snapshot{}, nil }

func (r *fakeRepo) state() (int, bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pages), r.published, r.failed
}

func TestCollectorPublishesOnlyAfterEveryPage(t *testing.T) {
	g := &fakeGateway{pages: 9}
	r := &fakeRepo{}
	c, _ := NewCollector(g, r, WithCollectorWorkers(4))
	if _, err := c.Collect(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	pages, published, failed := r.state()
	if g.calls.Load() != 9 || pages != 9 || !published || failed != 0 {
		t.Fatalf("calls=%d pages=%d published=%v failed=%d", g.calls.Load(), pages, published, failed)
	}
	if g.maxActive.Load() > 4 {
		t.Fatalf("max active requests %d exceeds worker bound", g.maxActive.Load())
	}
}

func TestCollectorProbesPageOneBeforeWorkers(t *testing.T) {
	block := make(chan struct{})
	g := &fakeGateway{pages: 5, block: block}
	r := &fakeRepo{}
	c, _ := NewCollector(g, r, WithCollectorWorkers(2))
	done := make(chan error, 1)
	go func() { _, err := c.Collect(context.Background(), 1); done <- err }()
	deadline := time.After(time.Second)
	for g.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatal("workers did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if pages, _, _ := r.state(); pages != 1 {
		t.Fatalf("page one was not persisted before workers: pages=%d", pages)
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCollectorFailureCancelsAndDoesNotPublish(t *testing.T) {
	block := make(chan struct{})
	g := &fakeGateway{pages: 20, failAt: 2, block: block}
	r := &fakeRepo{}
	c, _ := NewCollector(g, r, WithCollectorWorkers(4))
	if _, err := c.Collect(context.Background(), 1); err == nil {
		t.Fatal("expected error")
	}
	_, published, failed := r.state()
	if published || failed != 1 {
		t.Fatalf("published=%v failed=%d", published, failed)
	}
	if g.calls.Load() >= 20 {
		t.Fatalf("failure did not cancel scheduling: calls=%d", g.calls.Load())
	}
}

func TestCollectorRejectsPageCountChange(t *testing.T) {
	g := &fakeGateway{pages: 4, changedAt: 3}
	r := &fakeRepo{}
	c, _ := NewCollector(g, r)
	if _, err := c.Collect(context.Background(), 1); err == nil {
		t.Fatal("expected page count change failure")
	}
	_, published, failed := r.state()
	if published || failed != 1 {
		t.Fatalf("published=%v failed=%d", published, failed)
	}
}

func TestCollectorOptions(t *testing.T) {
	if _, err := NewCollector(&fakeGateway{}, &fakeRepo{}, WithCollectorWorkers(0)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid worker count, got %v", err)
	}
	c, err := NewCollector(&fakeGateway{}, &fakeRepo{})
	if err != nil || c.workers != DefaultCollectorWorkers {
		t.Fatalf("workers=%d err=%v", c.workers, err)
	}
}

func TestValidatePageBoundsResources(t *testing.T) {
	orders := make([]Order, MaxOrdersPerPage+1)
	if !errors.Is(validatePage(1, 1, orders), ErrInvalidInput) {
		t.Fatal("expected oversized page rejection")
	}
	if !errors.Is(validatePage(1, MaxPagesPerBatch+1, nil), ErrInvalidInput) {
		t.Fatal("expected page count rejection")
	}
}

func BenchmarkNormalizeOrders(b *testing.B) {
	orders := make([]esi.MarketOrder, MaxOrdersPerPage)
	for i := range orders {
		orders[i] = esi.MarketOrder{OrderID: int64(i + 1), TypeID: 34, LocationID: 60003760, SystemID: 30000142, Price: 5, VolumeTotal: 10, VolumeRemain: 9, MinVolume: 1, Range: "region", Duration: 90, Issued: time.Unix(1, 0)}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = normalizeOrders(orders)
	}
}
