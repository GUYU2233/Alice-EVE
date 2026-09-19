package esisync

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresJobRepository struct{ Pool *pgxpool.Pool }

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func NewPostgresJobRepository(p *pgxpool.Pool) (*PostgresJobRepository, error) {
	if p == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &PostgresJobRepository{p}, nil
}
func (r *PostgresJobRepository) Enqueue(ctx context.Context, a, k string, at time.Time) error {
	_, e := r.Pool.Exec(ctx, `INSERT INTO eve_sync_jobs(id,account_id,kind,run_after) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,kind) DO UPDATE SET status='pending',run_after=LEAST(eve_sync_jobs.run_after,EXCLUDED.run_after),last_error_code=NULL,claim_token=NULL,lease_until=NULL,updated_at=now()`, newID(), a, k, at)
	return e
}
func (r *PostgresJobRepository) Claim(ctx context.Context, now time.Time, lease time.Duration) (Job, error) {
	token := newID()
	var j Job
	e := r.Pool.QueryRow(ctx, `WITH candidate AS (SELECT id FROM eve_sync_jobs WHERE ((status IN ('pending','failed') AND run_after<=$1) OR (status='running' AND lease_until<=$1)) ORDER BY run_after,created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE eve_sync_jobs j SET status='running',attempts=j.attempts+1,claimed_at=$1,lease_until=$2,claim_token=$3,updated_at=$1 FROM candidate c WHERE j.id=c.id RETURNING j.id::text,j.account_id::text,j.kind,j.status,j.run_after,j.attempts,j.claim_token`, now, now.Add(lease), token).Scan(&j.ID, &j.AccountID, &j.Kind, &j.Status, &j.RunAfter, &j.Attempts, &j.ClaimToken)
	if errors.Is(e, pgx.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	return j, e
}
func (r *PostgresJobRepository) Complete(ctx context.Context, id, t string) error {
	return r.finish(ctx, id, t, "succeeded", time.Time{}, "")
}
func (r *PostgresJobRepository) Reschedule(ctx context.Context, id, t string, at time.Time) error {
	return r.finish(ctx, id, t, "pending", at, "")
}
func (r *PostgresJobRepository) Retry(ctx context.Context, id, t string, at time.Time, code string) error {
	return r.finish(ctx, id, t, "failed", at, code)
}
func (r *PostgresJobRepository) Block(ctx context.Context, id, t, code string) error {
	return r.finish(ctx, id, t, "blocked", time.Time{}, code)
}
func (r *PostgresJobRepository) finish(ctx context.Context, id, t, status string, at time.Time, code string) error {
	tag, e := r.Pool.Exec(ctx, `UPDATE eve_sync_jobs SET status=$3,run_after=CASE WHEN $4::timestamptz IS NULL THEN run_after ELSE $4 END,last_error_code=NULLIF($5,''),claim_token=NULL,lease_until=NULL,updated_at=now() WHERE id=$1 AND claim_token=$2 AND status='running'`, id, t, status, nullableTime(at), code)
	if e == nil && tag.RowsAffected() != 1 {
		return errors.New("sync job lease lost")
	}
	return e
}
func (r *PostgresJobRepository) ListAccount(ctx context.Context, accountID string) ([]Job, error) {
	if accountID == "" {
		return nil, errors.New("account ID is required")
	}
	rows, err := r.Pool.Query(ctx, `SELECT id::text,account_id::text,kind,status,run_after,attempts,COALESCE(claim_token,''),COALESCE(last_error_code,'') FROM eve_sync_jobs WHERE account_id=$1 ORDER BY kind,created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Job, 0)
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.AccountID, &j.Kind, &j.Status, &j.RunAfter, &j.Attempts, &j.ClaimToken, &j.LastErrorCode); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

var _ JobRepository = (*PostgresJobRepository)(nil)
