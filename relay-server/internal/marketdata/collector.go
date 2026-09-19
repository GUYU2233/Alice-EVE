package marketdata

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"relay-server/internal/esi"
)

const DefaultCollectorWorkers = 4

type PageGateway interface {
	RegionOrdersPage(context.Context, int64, int) ([]esi.MarketOrder, esi.Response, error)
}

type CollectorOption func(*Collector) error

// WithCollectorWorkers configures the maximum number of pages fetched and
// persisted concurrently after page one has established the page count.
func WithCollectorWorkers(workers int) CollectorOption {
	return func(c *Collector) error {
		if workers < 1 || workers > MaxPagesPerBatch {
			return fmt.Errorf("%w: collector workers", ErrInvalidInput)
		}
		c.workers = workers
		return nil
	}
}

type Collector struct {
	gateway PageGateway
	repo    Repository
	now     func() time.Time
	workers int
}

func NewCollector(gateway PageGateway, repo Repository, options ...CollectorOption) (*Collector, error) {
	if gateway == nil || repo == nil {
		return nil, errors.New("market gateway and repository are required")
	}
	c := &Collector{gateway: gateway, repo: repo, now: time.Now, workers: DefaultCollectorWorkers}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: nil collector option", ErrInvalidInput)
		}
		if err := option(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Collect probes page one to establish a stable page count, then streams the
// remaining pages through a bounded worker pool. Each worker persists its page
// before accepting another, bounding live page data to the worker count. Any
// fetch or append failure cancels the other workers and prevents publication.
func (c *Collector) Collect(ctx context.Context, regionID int64) (snapshot Snapshot, err error) {
	if !validRegion(regionID) {
		return Snapshot{}, ErrInvalidInput
	}
	batch, err := c.repo.BeginBatch(ctx, regionID, c.now().UTC())
	if err != nil {
		return Snapshot{}, err
	}
	published := false
	defer func() {
		if err != nil && !published {
			_ = c.repo.Fail(context.WithoutCancel(ctx), batch.ID, err.Error())
		}
	}()

	orders, meta, err := c.gateway.RegionOrdersPage(ctx, regionID, 1)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch region %d page 1: %w", regionID, err)
	}
	if meta.Pages < 1 || meta.Pages > MaxPagesPerBatch {
		return Snapshot{}, fmt.Errorf("%w: ESI page count", ErrInvalidInput)
	}
	expected := meta.Pages
	if err = c.repo.AppendPage(ctx, batch.ID, 1, expected, normalizeOrders(orders)); err != nil {
		return Snapshot{}, fmt.Errorf("append region %d page 1: %w", regionID, err)
	}

	if expected > 1 {
		workCtx, cancel := context.WithCancel(ctx)
		jobs := make(chan int)
		errCh := make(chan error, 1)
		workerCount := min(c.workers, expected-1)
		var wg sync.WaitGroup
		wg.Add(workerCount)
		for range workerCount {
			go func() {
				defer wg.Done()
				for page := range jobs {
					pageOrders, pageMeta, fetchErr := c.gateway.RegionOrdersPage(workCtx, regionID, page)
					if fetchErr != nil {
						reportCollectorError(errCh, cancel, fmt.Errorf("fetch region %d page %d: %w", regionID, page, fetchErr))
						return
					}
					if pageMeta.Pages != expected {
						reportCollectorError(errCh, cancel, fmt.Errorf("market page count changed from %d to %d", expected, pageMeta.Pages))
						return
					}
					if appendErr := c.repo.AppendPage(workCtx, batch.ID, page, expected, normalizeOrders(pageOrders)); appendErr != nil {
						reportCollectorError(errCh, cancel, fmt.Errorf("append region %d page %d: %w", regionID, page, appendErr))
						return
					}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for page := 2; page <= expected; page++ {
				select {
				case jobs <- page:
				case <-workCtx.Done():
					return
				}
			}
		}()
		wg.Wait()
		cancel()
		select {
		case err = <-errCh:
			return Snapshot{}, err
		default:
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Snapshot{}, ctxErr
		}
	}

	completedAt := c.now().UTC()
	expires := meta.ExpiresAt
	if !expires.IsZero() && expires.Before(completedAt) {
		expires = time.Time{}
	}
	snapshot, err = c.repo.Publish(ctx, batch.ID, completedAt, expires, meta.ETag)
	if err == nil {
		published = true
	}
	return snapshot, err
}

func reportCollectorError(errCh chan<- error, cancel context.CancelFunc, err error) {
	select {
	case errCh <- err:
		cancel()
	default:
	}
}

func normalizeOrders(orders []esi.MarketOrder) []Order {
	normalized := make([]Order, len(orders))
	for i, o := range orders {
		normalized[i] = Order{OrderID: o.OrderID, TypeID: o.TypeID, LocationID: o.LocationID, SystemID: o.SystemID, IsBuyOrder: o.IsBuyOrder, Price: o.Price, VolumeTotal: o.VolumeTotal, VolumeRemain: o.VolumeRemain, MinVolume: o.MinVolume, Range: o.Range, Duration: o.Duration, Issued: o.Issued}
	}
	return normalized
}
