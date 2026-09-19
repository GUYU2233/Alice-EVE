package marketdata

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) EnqueueDue(ctx context.Context, now time.Time, limit int) (int64, error) {
	if now.IsZero() || limit < 1 || limit > MaxClaimBatch {
		return 0, ErrInvalidJob
	}
	tag, err := r.pool.Exec(ctx, `WITH due AS (
		SELECT region_id,priority FROM region_market_schedule
		WHERE enabled AND next_run_at<=$1 AND (backoff_until IS NULL OR backoff_until<=$1)
		ORDER BY priority DESC,next_run_at,region_id LIMIT $2 FOR UPDATE SKIP LOCKED
	), inserted AS (
		INSERT INTO region_market_collection_jobs(region_id,priority,trigger,requested_at,created_at,updated_at)
		SELECT region_id,priority,'scheduler',$1,$1,$1 FROM due
		ON CONFLICT(region_id) WHERE state IN ('queued','claimed','collecting','publishing') DO NOTHING RETURNING region_id
	) UPDATE region_market_schedule s SET last_requested_at=$1,updated_at=$1
	FROM inserted i WHERE s.region_id=i.region_id`, now, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *PostgresRepository) MarkScheduleSuccess(ctx context.Context, regionID int64, at time.Time) error {
	if !validRegion(regionID) || at.IsZero() {
		return ErrInvalidJob
	}
	var tier string
	var refresh, minimum int
	err := r.pool.QueryRow(ctx, `SELECT tier,refresh_interval_seconds,min_refresh_interval_seconds FROM region_market_schedule WHERE region_id=$1`, regionID).Scan(&tier, &refresh, &minimum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	interval := time.Duration(refresh) * time.Second
	if interval <= 0 {
		interval = tierInterval(tier)
	}
	next := nextRun(regionID, at, interval, 0)
	minNext := at.Add(time.Duration(minimum) * time.Second)
	if next.Before(minNext) {
		next = minNext
	}
	_, err = r.pool.Exec(ctx, `UPDATE region_market_schedule SET last_success_at=$2,next_run_at=$3,consecutive_failures=0,backoff_until=NULL,updated_at=$2 WHERE region_id=$1`, regionID, at, next)
	return err
}

func (r *PostgresRepository) MarkScheduleFailure(ctx context.Context, regionID int64, at time.Time) error {
	if !validRegion(regionID) || at.IsZero() {
		return ErrInvalidJob
	}
	var failures int
	err := r.pool.QueryRow(ctx, `SELECT consecutive_failures FROM region_market_schedule WHERE region_id=$1`, regionID).Scan(&failures)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	backoff := at.Add(failureBackoff(failures + 1))
	_, err = r.pool.Exec(ctx, `UPDATE region_market_schedule SET consecutive_failures=consecutive_failures+1,backoff_until=$2,next_run_at=$2,updated_at=$3 WHERE region_id=$1`, regionID, backoff, at)
	return err
}

func (r *PostgresRepository) RegionStatus(ctx context.Context, regionID int64) (RegionStatus, error) {
	if !validRegion(regionID) {
		return RegionStatus{}, ErrInvalidJob
	}
	var out RegionStatus
	var refresh, minimum, stale int
	var requested, success, backoff *time.Time
	err := r.pool.QueryRow(ctx, `SELECT region_id,enabled,tier,priority,refresh_interval_seconds,min_refresh_interval_seconds,max_staleness_seconds,last_requested_at,next_run_at,last_success_at,consecutive_failures,backoff_until,access_score,change_score FROM region_market_schedule WHERE region_id=$1`, regionID).Scan(
		&out.Schedule.RegionID, &out.Schedule.Enabled, &out.Schedule.Tier, &out.Schedule.Priority, &refresh, &minimum, &stale, &requested, &out.Schedule.NextRunAt, &success, &out.Schedule.ConsecutiveFailures, &backoff, &out.Schedule.AccessScore, &out.Schedule.ChangeScore)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if err == nil {
		out.Schedule.RefreshInterval = time.Duration(refresh) * time.Second
		out.Schedule.MinRefreshInterval = time.Duration(minimum) * time.Second
		out.Schedule.MaxStaleness = time.Duration(stale) * time.Second
		if requested != nil {
			out.Schedule.LastRequestedAt = *requested
		}
		if success != nil {
			out.Schedule.LastSuccessAt = *success
		}
		if backoff != nil {
			out.Schedule.BackoffUntil = *backoff
		}
	}
	job, jobErr := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM region_market_collection_jobs WHERE region_id=$1 ORDER BY requested_at DESC,id DESC LIMIT 1`, regionID))
	if jobErr == nil {
		out.Job = &job
	} else if !errors.Is(jobErr, pgx.ErrNoRows) {
		return out, jobErr
	}
	snapshot, snapErr := r.Snapshot(ctx, regionID)
	if snapErr == nil {
		out.Snapshot = &snapshot
	} else if !errors.Is(snapErr, ErrNotFound) {
		return out, snapErr
	}
	if err != nil && out.Job == nil && out.Snapshot == nil {
		return out, ErrNotFound
	}
	return out, nil
}

var _ SchedulerRepository = (*PostgresRepository)(nil)
