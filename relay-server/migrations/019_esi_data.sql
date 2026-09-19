-- Normalized ESI snapshot and public cache persistence.
CREATE TABLE IF NOT EXISTS character_snapshots (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  character_id BIGINT NOT NULL CHECK (character_id > 0),
  domain TEXT NOT NULL CHECK (btrim(domain) <> ''),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) IN ('object', 'array')),
  fetched_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  stale BOOLEAN NOT NULL DEFAULT FALSE,
  source TEXT NOT NULL CHECK (btrim(source) <> ''),
  etag TEXT,
  PRIMARY KEY (account_id, character_id, domain),
  CHECK (expires_at >= fetched_at)
);
CREATE INDEX IF NOT EXISTS character_snapshots_account_domain_idx
  ON character_snapshots(account_id, domain, character_id);
CREATE INDEX IF NOT EXISTS character_snapshots_expiry_idx
  ON character_snapshots(expires_at) WHERE stale = FALSE;

-- Defense in depth for connections that set app.account_id. Repository queries
-- still include account_id explicitly, so isolation does not depend on RLS alone.
ALTER TABLE character_snapshots ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
  CREATE POLICY character_snapshots_account_isolation ON character_snapshots
    USING (account_id = NULLIF(current_setting('app.account_id', TRUE), '')::UUID)
    WITH CHECK (account_id = NULLIF(current_setting('app.account_id', TRUE), '')::UUID);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS public_data_cache (
  kind TEXT NOT NULL CHECK (btrim(kind) <> ''),
  cache_key TEXT NOT NULL CHECK (btrim(cache_key) <> ''),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) IN ('object', 'array')),
  fetched_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  etag TEXT,
  PRIMARY KEY (kind, cache_key),
  CHECK (expires_at >= fetched_at)
);
CREATE INDEX IF NOT EXISTS public_data_cache_expiry_idx
  ON public_data_cache(expires_at);
