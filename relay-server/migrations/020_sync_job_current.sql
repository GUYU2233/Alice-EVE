-- Keep exactly one current synchronization job for each account/domain.
-- Older terminal rows created before recurring scheduling remain historical noise
-- and are removed before replacing the partial active-only uniqueness rule.
WITH ranked AS (
  SELECT id, row_number() OVER (
    PARTITION BY account_id, kind
    ORDER BY updated_at DESC, created_at DESC, id DESC
  ) AS rn
  FROM eve_sync_jobs
)
DELETE FROM eve_sync_jobs j
USING ranked r
WHERE j.id = r.id AND r.rn > 1;

DROP INDEX IF EXISTS eve_sync_jobs_active_kind_idx;
CREATE UNIQUE INDEX IF NOT EXISTS eve_sync_jobs_account_kind_idx
  ON eve_sync_jobs(account_id, kind);
