package marketdata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const jobColumns = `id::text,region_id,state,priority,trigger,COALESCE(requested_by_account_id::text,''),COALESCE(claim_token::text,''),requested_at,claimed_at,claim_expires_at,started_at,completed_at,attempt,COALESCE(failure_code,''),COALESCE(failure_detail,'')`

func (r *PostgresRepository) Enqueue(ctx context.Context, regionID int64, priority int, trigger JobTrigger, accountID string, requestedAt time.Time) (CollectionJob, error) {
	if err := validateEnqueue(regionID, priority, trigger, accountID, requestedAt); err != nil {
		return CollectionJob{}, err
	}
	var row pgx.Row
	if accountID == "" {
		row = r.pool.QueryRow(ctx, `INSERT INTO region_market_collection_jobs(region_id,priority,trigger,requested_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$4,$4) ON CONFLICT(region_id) WHERE state IN ('queued','claimed','collecting','publishing') DO NOTHING RETURNING `+jobColumns, regionID, priority, trigger, requestedAt)
	} else {
		row = r.pool.QueryRow(ctx, `INSERT INTO region_market_collection_jobs(region_id,priority,trigger,requested_by_account_id,requested_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$5,$5) ON CONFLICT(region_id) WHERE state IN ('queued','claimed','collecting','publishing') DO NOTHING RETURNING `+jobColumns, regionID, priority, trigger, accountID, requestedAt)
	}
	job, err := scanJob(row)
	if !errors.Is(err, pgx.ErrNoRows) {
		return job, err
	}
	// Idempotent enqueue: return the database mutex holder rather than creating
	// another active job for the region.
	return scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM region_market_collection_jobs WHERE region_id=$1 AND state IN ('queued','claimed','collecting','publishing')`, regionID))
}

func (r *PostgresRepository) Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]CollectionJob, error) {
	if now.IsZero() || !validLease(lease) || limit < 1 || limit > MaxClaimBatch {
		return nil, ErrInvalidJob
	}
	rows, err := r.pool.Query(ctx, `WITH candidates AS (
		SELECT id FROM region_market_collection_jobs WHERE state='queued'
		ORDER BY priority DESC,requested_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE region_market_collection_jobs j SET state='claimed',claim_token=gen_random_uuid(),claim_expires_at=$3,
		claimed_at=$2,attempt=j.attempt+1,updated_at=$2 FROM candidates c WHERE j.id=c.id RETURNING j.*
	) SELECT `+jobColumns+` FROM claimed ORDER BY priority DESC,requested_at,id`, limit, now, now.Add(lease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]CollectionJob, 0, limit)
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *PostgresRepository) RenewLease(ctx context.Context, id, token string, now time.Time, lease time.Duration) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" || now.IsZero() || !validLease(lease) {
		return ErrInvalidJob
	}
	tag, err := r.pool.Exec(ctx, `UPDATE region_market_collection_jobs SET claim_expires_at=$4,updated_at=$3
		WHERE id=$1 AND claim_token=$2 AND state IN ('claimed','collecting','publishing') AND claim_expires_at>$3`, id, token, now, now.Add(lease))
	return claimedResult(tag.RowsAffected(), err)
}

func (r *PostgresRepository) CompleteJob(ctx context.Context, id, token string, completedAt time.Time) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" || completedAt.IsZero() {
		return ErrInvalidJob
	}
	tag, err := r.pool.Exec(ctx, `UPDATE region_market_collection_jobs SET state='complete',completed_at=$3,claim_token=NULL,claim_expires_at=NULL,failure_code=NULL,failure_detail=NULL,updated_at=$3
		WHERE id=$1 AND claim_token=$2 AND state IN ('claimed','collecting','publishing') AND claim_expires_at>$3`, id, token, completedAt)
	return claimedResult(tag.RowsAffected(), err)
}

func (r *PostgresRepository) FailJob(ctx context.Context, id, token string, completedAt time.Time, code, detail string) error {
	code, detail = strings.TrimSpace(code), strings.TrimSpace(detail)
	if strings.TrimSpace(id) == "" || strings.TrimSpace(token) == "" || completedAt.IsZero() || code == "" || len(code) > MaxJobFailureCode || detail == "" {
		return ErrInvalidJob
	}
	if len(detail) > MaxJobFailureDetail {
		detail = detail[:MaxJobFailureDetail]
	}
	tag, err := r.pool.Exec(ctx, `UPDATE region_market_collection_jobs SET state='failed',completed_at=$3,claim_token=NULL,claim_expires_at=NULL,failure_code=$4,failure_detail=$5,updated_at=$3
		WHERE id=$1 AND claim_token=$2 AND state IN ('claimed','collecting','publishing') AND claim_expires_at>$3`, id, token, completedAt, code, detail)
	return claimedResult(tag.RowsAffected(), err)
}

func (r *PostgresRepository) RecoverExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if now.IsZero() || limit < 1 || limit > MaxClaimBatch {
		return 0, ErrInvalidJob
	}
	tag, err := r.pool.Exec(ctx, `WITH expired AS (
		SELECT id FROM region_market_collection_jobs WHERE state IN ('claimed','collecting','publishing') AND claim_expires_at<=$1
		ORDER BY claim_expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
	) UPDATE region_market_collection_jobs j SET state='failed',completed_at=$1,claim_token=NULL,claim_expires_at=NULL,
	failure_code='claim_expired',failure_detail='worker claim lease expired',updated_at=$1 FROM expired e WHERE j.id=e.id`, now, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func claimedResult(affected int64, err error) error {
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrClaimLost
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (CollectionJob, error) {
	var j CollectionJob
	var claimedAt, expiresAt, startedAt, completedAt *time.Time
	err := row.Scan(&j.ID, &j.RegionID, &j.State, &j.Priority, &j.Trigger, &j.RequestedByAccountID, &j.ClaimToken,
		&j.RequestedAt, &claimedAt, &expiresAt, &startedAt, &completedAt, &j.Attempt, &j.FailureCode, &j.FailureDetail)
	if claimedAt != nil {
		j.ClaimedAt = *claimedAt
	}
	if expiresAt != nil {
		j.ClaimExpiresAt = *expiresAt
	}
	if startedAt != nil {
		j.StartedAt = *startedAt
	}
	if completedAt != nil {
		j.CompletedAt = *completedAt
	}
	return j, err
}

var _ JobRepository = (*PostgresRepository)(nil)
