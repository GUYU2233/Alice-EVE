-- Durable, revisioned market planning jobs. HTTP requests create/read jobs;
-- workers may continue optimization after the requesting client disconnects.
CREATE TABLE market_plan_jobs (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  character_id BIGINT NOT NULL DEFAULT 0 CHECK (character_id >= 0),
  mode TEXT NOT NULL CHECK (mode IN ('single','basket','chain')),
  source_region_ids BIGINT[] NOT NULL CHECK (cardinality(source_region_ids) BETWEEN 1 AND 64),
  destination_scope TEXT NOT NULL DEFAULT 'all_collected_regions' CHECK (destination_scope IN ('all_collected_regions','selected_regions')),
  destination_region_ids BIGINT[] NOT NULL DEFAULT '{}',
  constraints JSONB NOT NULL,
  constraint_hash TEXT NOT NULL CHECK (length(constraint_hash)=64),
  snapshot_signature TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','discovering','routing','optimizing','watching','completed','failed','cancelled')),
  progress JSONB NOT NULL DEFAULT '{}',
  iteration BIGINT NOT NULL DEFAULT 0 CHECK (iteration >= 0),
  result_revision BIGINT NOT NULL DEFAULT 0 CHECK (result_revision >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  worker_token UUID,
  lease_until TIMESTAMPTZ,
  attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);
CREATE INDEX market_plan_jobs_account_updated_idx ON market_plan_jobs(account_id,updated_at DESC);
CREATE INDEX market_plan_jobs_queue_idx ON market_plan_jobs(state,lease_until,updated_at) WHERE state IN ('queued','discovering','routing','optimizing');
CREATE UNIQUE INDEX market_plan_jobs_active_dedupe_idx ON market_plan_jobs(account_id,character_id,mode,constraint_hash) WHERE state IN ('queued','discovering','routing','optimizing','watching');

CREATE TABLE market_plan_results (
  job_id UUID NOT NULL REFERENCES market_plan_jobs(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  rank INTEGER NOT NULL CHECK (rank > 0),
  stable_key TEXT NOT NULL CHECK (length(stable_key) BETWEEN 1 AND 512),
  score DOUBLE PRECISION NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(job_id,revision,stable_key)
);
CREATE INDEX market_plan_results_latest_idx ON market_plan_results(job_id,revision DESC,rank);

CREATE TABLE market_plan_frontiers (
  job_id UUID PRIMARY KEY REFERENCES market_plan_jobs(id) ON DELETE CASCADE,
  iteration BIGINT NOT NULL CHECK (iteration >= 0),
  payload JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
