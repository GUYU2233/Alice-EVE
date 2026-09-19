package marketplan

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(p *pgxpool.Pool) (*PostgresRepository, error) {
	if p == nil {
		return nil, errors.New("postgres pool required")
	}
	return &PostgresRepository{p}, nil
}

const jobColumns = `id::text,account_id::text,character_id,mode,source_region_ids,destination_scope,destination_region_ids,constraints,constraint_hash,snapshot_signature,state,progress,iteration,result_revision,last_error,created_at,updated_at,started_at,completed_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	var c, progress []byte
	err := row.Scan(&j.ID, &j.AccountID, &j.CharacterID, &j.Mode, &j.SourceRegionIDs, &j.DestinationScope, &j.DestinationRegionIDs, &c, &j.ConstraintHash, &j.SnapshotSignature, &j.State, &progress, &j.Iteration, &j.ResultRevision, &j.LastError, &j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt)
	if err != nil {
		return j, err
	}
	if err = json.Unmarshal(c, &j.Constraints); err != nil {
		return j, err
	}
	j.Progress = append(json.RawMessage(nil), progress...)
	return j, nil
}
func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
func (r *PostgresRepository) Create(ctx context.Context, account string, q CreateRequest, hash string) (Job, error) {
	c, _ := json.Marshal(q.Constraints)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	// A workspace has exactly one live plan per account/character/mode. Without
	// superseding older constraints, every historic plan watches every snapshot
	// forever and multiplies expensive BASKET/CHAIN work.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text||':'||$2::text||':'||$3::text,0))`, account, q.CharacterID, q.Mode); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE market_plan_jobs SET state='cancelled',worker_token=NULL,lease_until=NULL,completed_at=now(),updated_at=now() WHERE account_id=$1::uuid AND character_id=$2 AND mode=$3 AND state IN ('queued','discovering','routing','optimizing','watching')`, account, q.CharacterID, q.Mode); err != nil {
		return Job{}, err
	}
	job, err := scanJob(tx.QueryRow(ctx, `INSERT INTO market_plan_jobs(id,account_id,character_id,mode,source_region_ids,destination_scope,destination_region_ids,constraints,constraint_hash) VALUES(gen_random_uuid(),$1::uuid,$2,$3,$4,$5,$6,$7,$8) RETURNING `+jobColumns, account, q.CharacterID, q.Mode, q.SourceRegionIDs, q.DestinationScope, nonNilIDs(q.DestinationRegionIDs), c, hash))
	if err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}
func (r *PostgresRepository) Get(ctx context.Context, account, id string) (Job, error) {
	j, e := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM market_plan_jobs WHERE id=$1::uuid AND account_id=$2::uuid`, id, account))
	if errors.Is(e, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, e
}
func (r *PostgresRepository) ResultsAfter(ctx context.Context, account, id string, after int64) ([]Result, error) {
	rows, e := r.pool.Query(ctx, `SELECT r.revision,r.rank,r.stable_key,r.score,r.payload,r.created_at FROM market_plan_results r JOIN market_plan_jobs j ON j.id=r.job_id WHERE r.job_id=$1::uuid AND j.account_id=$2::uuid AND r.revision=(SELECT max(revision) FROM market_plan_results WHERE job_id=r.job_id AND revision>$3) ORDER BY r.rank`, id, account, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		var x Result
		if e = rows.Scan(&x.Revision, &x.Rank, &x.StableKey, &x.Score, &x.Payload, &x.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) RequeueWatching(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE market_plan_jobs j SET state='queued',snapshot_signature=(SELECT max(s.published_at)::text FROM region_market_snapshots s WHERE s.region_id=ANY(j.source_region_ids||j.destination_region_ids) OR j.destination_scope='all_collected_regions'),updated_at=now() WHERE state='watching' AND EXISTS (SELECT 1 FROM region_market_snapshots s WHERE (s.region_id=ANY(j.source_region_ids||j.destination_region_ids) OR j.destination_scope='all_collected_regions') AND s.published_at>COALESCE(NULLIF(j.snapshot_signature,'')::timestamptz,'epoch'::timestamptz))`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *PostgresRepository) Claim(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Claim, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := r.pool.Query(ctx, `WITH picked AS (SELECT id FROM market_plan_jobs WHERE state IN ('queued','discovering','routing','optimizing') AND (lease_until IS NULL OR lease_until<$1) ORDER BY updated_at,id LIMIT $2 FOR UPDATE SKIP LOCKED) UPDATE market_plan_jobs SET worker_token=gen_random_uuid(),lease_until=$1+$3::interval,attempt=attempt+1,state=CASE WHEN state='queued' THEN 'discovering' ELSE state END,started_at=COALESCE(started_at,$1),updated_at=$1 WHERE id IN (SELECT id FROM picked) RETURNING `+jobColumns+`,worker_token::text,lease_until`, now, limit, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Claim{}
	for rows.Next() {
		var c Claim
		var raw, progress []byte
		if err = rows.Scan(&c.Job.ID, &c.Job.AccountID, &c.Job.CharacterID, &c.Job.Mode, &c.Job.SourceRegionIDs, &c.Job.DestinationScope, &c.Job.DestinationRegionIDs, &raw, &c.Job.ConstraintHash, &c.Job.SnapshotSignature, &c.Job.State, &progress, &c.Job.Iteration, &c.Job.ResultRevision, &c.Job.LastError, &c.Job.CreatedAt, &c.Job.UpdatedAt, &c.Job.StartedAt, &c.Job.CompletedAt, &c.Token, &c.LeaseUntil); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &c.Job.Constraints); err != nil {
			return nil, err
		}
		c.Job.Progress = progress
		out = append(out, c)
	}
	return out, rows.Err()
}
func pendingKeys(x []PendingResult) []string {
	out := make([]string, len(x))
	for i := range x {
		out[i] = x[i].StableKey
	}
	return out
}
func (r *PostgresRepository) Commit(ctx context.Context, claim Claim, in Commit, now time.Time) (Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	err = tx.QueryRow(ctx, `UPDATE market_plan_jobs SET state=$3,progress=$4,iteration=iteration+1,result_revision=result_revision+CASE WHEN cardinality($5::text[])>0 THEN 1 ELSE 0 END,worker_token=NULL,lease_until=NULL,updated_at=$6 WHERE id=$1::uuid AND worker_token=$2::uuid AND lease_until>=$6 RETURNING result_revision`, claim.Job.ID, claim.Token, in.State, []byte(in.Progress), pendingKeys(in.Results), now).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrLeaseLost
	}
	if err != nil {
		return Job{}, err
	}
	if len(in.Results) > 0 {
		batch := &pgx.Batch{}
		for i, x := range in.Results {
			batch.Queue(`INSERT INTO market_plan_results(job_id,revision,rank,stable_key,score,payload,created_at) VALUES($1::uuid,$2,$3,$4,$5,$6,$7)`, claim.Job.ID, revision, i+1, x.StableKey, x.Score, x.Payload, now)
		}
		br := tx.SendBatch(ctx, batch)
		for range in.Results {
			if _, err = br.Exec(); err != nil {
				_ = br.Close()
				return Job{}, err
			}
		}
		if err = br.Close(); err != nil {
			return Job{}, err
		}
	}
	if len(in.Frontier) > 0 {
		if _, err = tx.Exec(ctx, `INSERT INTO market_plan_frontiers(job_id,iteration,payload,updated_at) VALUES($1::uuid,$2,$3,$4) ON CONFLICT(job_id) DO UPDATE SET iteration=EXCLUDED.iteration,payload=EXCLUDED.payload,updated_at=EXCLUDED.updated_at`, claim.Job.ID, claim.Job.Iteration+1, []byte(in.Frontier), now); err != nil {
			return Job{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return r.Get(ctx, claim.Job.AccountID, claim.Job.ID)
}
func (r *PostgresRepository) Fail(ctx context.Context, claim Claim, message string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE market_plan_jobs SET state=CASE WHEN attempt>=5 THEN 'failed' ELSE 'queued' END,last_error=$3,worker_token=NULL,lease_until=NULL,updated_at=$4 WHERE id=$1::uuid AND worker_token=$2::uuid`, claim.Job.ID, claim.Token, message, now)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return err
}

func (r *PostgresRepository) Cancel(ctx context.Context, account, id string) (Job, error) {
	j, e := scanJob(r.pool.QueryRow(ctx, `UPDATE market_plan_jobs SET state='cancelled',updated_at=$3,completed_at=$3 WHERE id=$1::uuid AND account_id=$2::uuid AND state NOT IN ('completed','failed','cancelled') RETURNING `+jobColumns, id, account, time.Now().UTC()))
	if errors.Is(e, pgx.ErrNoRows) {
		return r.Get(ctx, account, id)
	}
	return j, e
}
