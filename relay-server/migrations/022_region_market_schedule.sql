-- Persistent market collection scheduling and leased jobs. The partial unique
-- index is the database-level same-region mutex across every active state.
CREATE TABLE region_market_schedule (
  region_id BIGINT PRIMARY KEY CHECK (region_id > 0 AND region_id <= 2147483647),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  tier TEXT NOT NULL DEFAULT 'D' CHECK (tier IN ('A','B','C','D')),
  priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN -1000000 AND 1000000),
  refresh_interval_seconds INTEGER NOT NULL CHECK (refresh_interval_seconds BETWEEN 60 AND 604800),
  min_refresh_interval_seconds INTEGER NOT NULL CHECK (min_refresh_interval_seconds BETWEEN 30 AND 604800),
  max_staleness_seconds INTEGER NOT NULL CHECK (max_staleness_seconds BETWEEN 60 AND 2592000),
  last_requested_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ NOT NULL,
  last_success_at TIMESTAMPTZ,
  consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures BETWEEN 0 AND 1000000),
  backoff_until TIMESTAMPTZ,
  access_score DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (access_score >= 0 AND access_score <= 1000000000),
  change_score DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (change_score >= 0 AND change_score <= 1000000000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (min_refresh_interval_seconds <= refresh_interval_seconds),
  CHECK (refresh_interval_seconds <= max_staleness_seconds)
);

CREATE INDEX region_market_schedule_due_idx
  ON region_market_schedule(next_run_at, priority DESC, region_id)
  WHERE enabled;

CREATE TABLE region_market_collection_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  region_id BIGINT NOT NULL CHECK (region_id > 0 AND region_id <= 2147483647),
  state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','claimed','collecting','publishing','complete','failed','canceled')),
  priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN -1000000 AND 1000000),
  trigger TEXT NOT NULL CHECK (trigger IN ('scheduler','user_priority_refresh','startup_recovery','manual_admin','stale_snapshot')),
  requested_by_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
  claim_token UUID,
  claim_expires_at TIMESTAMPTZ,
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  claimed_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt BETWEEN 0 AND 1000000),
  failure_code TEXT CHECK (failure_code IS NULL OR length(failure_code) BETWEEN 1 AND 64),
  failure_detail TEXT CHECK (failure_detail IS NULL OR length(failure_detail) BETWEEN 1 AND 2048),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((claim_token IS NULL) = (claim_expires_at IS NULL)),
  CHECK (state NOT IN ('claimed','collecting','publishing') OR (claim_token IS NOT NULL AND claimed_at IS NOT NULL)),
  CHECK (state NOT IN ('complete','failed','canceled') OR completed_at IS NOT NULL)
);

CREATE UNIQUE INDEX region_market_collection_jobs_active_region_uidx
  ON region_market_collection_jobs(region_id)
  WHERE state IN ('queued','claimed','collecting','publishing');
CREATE INDEX region_market_collection_jobs_claim_idx
  ON region_market_collection_jobs(priority DESC, requested_at, id)
  WHERE state = 'queued';
CREATE INDEX region_market_collection_jobs_expired_claim_idx
  ON region_market_collection_jobs(claim_expires_at, id)
  WHERE state IN ('claimed','collecting','publishing');
CREATE INDEX region_market_collection_jobs_region_history_idx
  ON region_market_collection_jobs(region_id, requested_at DESC);
