package marketdata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) BeginBatch(ctx context.Context, regionID int64, startedAt time.Time) (Batch, error) {
	if !validRegion(regionID) || startedAt.IsZero() {
		return Batch{}, ErrInvalidInput
	}
	var b Batch
	err := r.pool.QueryRow(ctx, `INSERT INTO region_market_batches(id,region_id,state,started_at)
		VALUES(gen_random_uuid(),$1,'collecting',$2)
		RETURNING id::text,region_id,state,started_at`, regionID, startedAt).Scan(&b.ID, &b.RegionID, &b.State, &b.StartedAt)
	return b, err
}

func (r *PostgresRepository) AppendPage(ctx context.Context, batchID string, page, expected int, orders []Order) error {
	if strings.TrimSpace(batchID) == "" {
		return ErrInvalidInput
	}
	if err := validatePage(page, expected, orders); err != nil {
		return fmt.Errorf("append page %d/%d: %w", page, expected, err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var regionID int64
	var state string
	var existingExpected *int
	if err = tx.QueryRow(ctx, `SELECT region_id,state,expected_pages FROM region_market_batches WHERE id=$1 FOR UPDATE`, batchID).Scan(&regionID, &state, &existingExpected); err != nil {
		return mapNoRows(err)
	}
	if state != "collecting" {
		return ErrBatchState
	}
	if existingExpected != nil && *existingExpected != expected {
		return ErrInvalidInput
	}
	var inserted int
	if err = tx.QueryRow(ctx, `INSERT INTO region_market_batch_pages(batch_id,page_number,order_count) VALUES($1,$2,$3)
		ON CONFLICT DO NOTHING RETURNING 1`, batchID, page, len(orders)).Scan(&inserted); errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: page already appended", ErrInvalidInput)
	} else if err != nil {
		return err
	}
	orderIDs := make([]int64, len(orders))
	typeIDs := make([]int64, len(orders))
	locationIDs := make([]int64, len(orders))
	systemIDs := make([]int64, len(orders))
	buyOrders := make([]bool, len(orders))
	prices := make([]float64, len(orders))
	volumeTotals := make([]int64, len(orders))
	volumeRemains := make([]int64, len(orders))
	minVolumes := make([]int64, len(orders))
	ranges := make([]string, len(orders))
	durations := make([]int32, len(orders))
	issued := make([]time.Time, len(orders))
	for i, o := range orders {
		orderIDs[i], typeIDs[i], locationIDs[i], systemIDs[i] = o.OrderID, o.TypeID, o.LocationID, o.SystemID
		buyOrders[i], prices[i] = o.IsBuyOrder, o.Price
		volumeTotals[i], volumeRemains[i], minVolumes[i] = o.VolumeTotal, o.VolumeRemain, o.MinVolume
		ranges[i], durations[i], issued[i] = o.Range, int32(o.Duration), o.Issued
	}
	var insertedOrders int64
	err = tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO region_market_orders(batch_id,order_id,region_id,type_id,location_id,system_id,is_buy_order,price,volume_total,volume_remain,min_volume,order_range,duration,issued)
		SELECT $1,u.order_id,$2,u.type_id,u.location_id,u.system_id,u.is_buy_order,u.price,u.volume_total,u.volume_remain,u.min_volume,u.order_range,u.duration,u.issued
		FROM unnest($3::bigint[],$4::bigint[],$5::bigint[],$6::bigint[],$7::boolean[],$8::double precision[],$9::bigint[],$10::bigint[],$11::bigint[],$12::text[],$13::integer[],$14::timestamptz[])
			AS u(order_id,type_id,location_id,system_id,is_buy_order,price,volume_total,volume_remain,min_volume,order_range,duration,issued)
		ON CONFLICT(batch_id,order_id) DO NOTHING RETURNING 1
	) SELECT count(*) FROM inserted`, batchID, regionID, orderIDs, typeIDs, locationIDs, systemIDs, buyOrders, prices, volumeTotals, volumeRemains, minVolumes, ranges, durations, issued).Scan(&insertedOrders)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE region_market_batches SET expected_pages=$2,collected_pages=collected_pages+1,order_count=order_count+$3 WHERE id=$1`, batchID, expected, insertedOrders)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Publish(ctx context.Context, batchID string, completedAt, expiresAt time.Time, etag string) (Snapshot, error) {
	if strings.TrimSpace(batchID) == "" || completedAt.IsZero() || (!expiresAt.IsZero() && expiresAt.Before(completedAt)) {
		return Snapshot{}, ErrInvalidInput
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback(ctx)
	var regionID int64
	var state string
	var expected *int
	var collected int
	if err = tx.QueryRow(ctx, `SELECT region_id,state,expected_pages,collected_pages FROM region_market_batches WHERE id=$1 FOR UPDATE`, batchID).Scan(&regionID, &state, &expected, &collected); err != nil {
		return Snapshot{}, mapNoRows(err)
	}
	if state != "collecting" {
		return Snapshot{}, ErrBatchState
	}
	if expected == nil || *expected != collected {
		return Snapshot{}, ErrIncompleteBatch
	}
	// Build the complete top-of-book summary before making this batch visible.
	// Any aggregation failure rolls back this transaction and leaves current_batch_id unchanged.
	if _, err = tx.Exec(ctx, buildBestLevelsSQL, batchID); err != nil {
		return Snapshot{}, fmt.Errorf("build best levels: %w", err)
	}
	if _, err = tx.Exec(ctx, buildDepthLevelsSQL, batchID); err != nil {
		return Snapshot{}, fmt.Errorf("build depth levels: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE region_market_batches SET state='complete',completed_at=$2,expires_at=NULLIF($3,'0001-01-01 00:00:00+00'::timestamptz),etag=NULLIF($4,'') WHERE id=$1`, batchID, completedAt, expiresAt, etag); err != nil {
		return Snapshot{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO region_market_snapshots(region_id,current_batch_id,previous_batch_id,published_at) VALUES($1,$2,NULL,$3)
		ON CONFLICT(region_id) DO UPDATE SET previous_batch_id=region_market_snapshots.current_batch_id,current_batch_id=EXCLUDED.current_batch_id,published_at=EXCLUDED.published_at`, regionID, batchID, completedAt); err != nil {
		return Snapshot{}, err
	}
	// Bound durable completed data to current+previous; collecting/failed rows remain independently auditable.
	if _, err = tx.Exec(ctx, `DELETE FROM region_market_batches b WHERE b.region_id=$1 AND b.state='complete' AND b.id NOT IN
		(SELECT current_batch_id FROM region_market_snapshots WHERE region_id=$1 UNION SELECT previous_batch_id FROM region_market_snapshots WHERE region_id=$1 AND previous_batch_id IS NOT NULL)`, regionID); err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Snapshot{}, err
	}
	return r.Snapshot(ctx, regionID)
}

const buildBestLevelsSQL = `
INSERT INTO region_market_best_levels(
 batch_id,region_id,location_id,system_id,type_id,best_ask,ask_depth,best_bid,bid_depth
)
WITH prices AS (
 SELECT batch_id,region_id,location_id,system_id,type_id,is_buy_order,price,
        SUM(volume_remain)::bigint AS depth
 FROM region_market_orders
 WHERE batch_id=$1 AND volume_remain>0
 GROUP BY batch_id,region_id,location_id,system_id,type_id,is_buy_order,price
), ranked AS (
 SELECT *,ROW_NUMBER() OVER (
   PARTITION BY batch_id,region_id,location_id,system_id,type_id,is_buy_order
   ORDER BY CASE WHEN is_buy_order THEN price END DESC,
            CASE WHEN NOT is_buy_order THEN price END ASC
 ) AS rn
 FROM prices
)
SELECT batch_id,region_id,location_id,system_id,type_id,
       MAX(price) FILTER (WHERE NOT is_buy_order),
       MAX(depth) FILTER (WHERE NOT is_buy_order),
       MAX(price) FILTER (WHERE is_buy_order),
       MAX(depth) FILTER (WHERE is_buy_order)
FROM ranked WHERE rn=1
GROUP BY batch_id,region_id,location_id,system_id,type_id
ON CONFLICT (batch_id,region_id,location_id,system_id,type_id) DO NOTHING`

const buildDepthLevelsSQL = `
INSERT INTO region_market_order_depth_levels(
 batch_id,region_id,location_id,system_id,type_id,is_buy_order,level_no,
 price,volume,cumulative_volume,minimum_volume,source_order_count
)
WITH prices AS (
 SELECT batch_id,region_id,location_id,system_id,type_id,is_buy_order,price,
        SUM(volume_remain)::bigint AS volume,
        MIN(GREATEST(min_volume,1))::bigint AS minimum_volume,
        COUNT(*)::integer AS source_order_count
 FROM region_market_orders
 WHERE batch_id=$1 AND volume_remain>0
 GROUP BY batch_id,region_id,location_id,system_id,type_id,is_buy_order,price
), ranked AS (
 SELECT *,ROW_NUMBER() OVER (
   PARTITION BY batch_id,region_id,location_id,system_id,type_id,is_buy_order
   ORDER BY CASE WHEN is_buy_order THEN price END DESC,
            CASE WHEN NOT is_buy_order THEN price END ASC
 ) AS level_no
 FROM prices
), bounded AS (
 SELECT *,SUM(volume) OVER (
   PARTITION BY batch_id,region_id,location_id,system_id,type_id,is_buy_order
   ORDER BY level_no ROWS UNBOUNDED PRECEDING
 )::bigint AS cumulative_volume
 FROM ranked WHERE level_no<=32
)
SELECT batch_id,region_id,location_id,system_id,type_id,is_buy_order,level_no,
       price,volume,cumulative_volume,minimum_volume,source_order_count
FROM bounded
ON CONFLICT (batch_id,region_id,location_id,system_id,type_id,is_buy_order,level_no) DO NOTHING`

func (r *PostgresRepository) Fail(ctx context.Context, batchID, failure string) error {
	failure = strings.TrimSpace(failure)
	if batchID == "" || failure == "" {
		return ErrInvalidInput
	}
	if len(failure) > 1024 {
		failure = failure[:1024]
	}
	tag, err := r.pool.Exec(ctx, `UPDATE region_market_batches SET state='failed',failure=$2 WHERE id=$1 AND state='collecting'`, batchID, failure)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBatchState
	}
	return nil
}

func (r *PostgresRepository) Snapshot(ctx context.Context, regionID int64) (Snapshot, error) {
	if !validRegion(regionID) {
		return Snapshot{}, ErrInvalidInput
	}
	var s Snapshot
	var prevID *string
	var cCompleted, cExpires *time.Time
	err := r.pool.QueryRow(ctx, `SELECT s.region_id,c.id::text,c.state,c.expected_pages,c.collected_pages,c.order_count,c.started_at,c.completed_at,c.expires_at,COALESCE(c.etag,''),s.previous_batch_id::text
		FROM region_market_snapshots s JOIN region_market_batches c ON c.id=s.current_batch_id WHERE s.region_id=$1`, regionID).Scan(&s.RegionID, &s.Current.ID, &s.Current.State, &s.Current.ExpectedPages, &s.Current.CollectedPages, &s.Current.OrderCount, &s.Current.StartedAt, &cCompleted, &cExpires, &s.Current.ETag, &prevID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}
	s.Current.RegionID = regionID
	if cCompleted != nil {
		s.Current.CompletedAt = *cCompleted
	}
	if cExpires != nil {
		s.Current.ExpiresAt = *cExpires
	}
	if prevID != nil {
		var b Batch
		var completed, expires *time.Time
		err = r.pool.QueryRow(ctx, `SELECT id::text,region_id,state,expected_pages,collected_pages,order_count,started_at,completed_at,expires_at,COALESCE(etag,'') FROM region_market_batches WHERE id=$1`, *prevID).Scan(&b.ID, &b.RegionID, &b.State, &b.ExpectedPages, &b.CollectedPages, &b.OrderCount, &b.StartedAt, &completed, &expires, &b.ETag)
		if err != nil {
			return Snapshot{}, err
		}
		if completed != nil {
			b.CompletedAt = *completed
		}
		if expires != nil {
			b.ExpiresAt = *expires
		}
		s.Previous = &b
	}
	return s, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

var _ Repository = (*PostgresRepository)(nil)
