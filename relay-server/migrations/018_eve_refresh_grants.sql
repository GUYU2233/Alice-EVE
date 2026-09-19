-- Encrypted EVE OAuth refresh grants and durable synchronization jobs.
CREATE TABLE IF NOT EXISTS eve_refresh_grants (
  account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  provider_subject TEXT NOT NULL UNIQUE,
  refresh_ciphertext BYTEA NOT NULL,
  refresh_nonce BYTEA NOT NULL,
  key_id TEXT NOT NULL,
  scopes TEXT[] NOT NULL DEFAULT '{}',
  access_expires_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
  revoked_at TIMESTAMPTZ,
  token_version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS eve_sync_jobs (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed','blocked')),
  run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error_code TEXT,
  claimed_at TIMESTAMPTZ,
  lease_until TIMESTAMPTZ,
  claim_token TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS eve_sync_jobs_active_kind_idx ON eve_sync_jobs(account_id,kind) WHERE status IN ('pending','running','failed');
CREATE INDEX IF NOT EXISTS eve_sync_jobs_ready_idx ON eve_sync_jobs(run_after, created_at) WHERE status IN ('pending','failed','running');
CREATE INDEX IF NOT EXISTS eve_sync_jobs_account_idx ON eve_sync_jobs(account_id, created_at DESC);
