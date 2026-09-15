-- Agent execution lifecycle, streamed events and bounded audit metadata.
CREATE TABLE IF NOT EXISTS agent_runs (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  input_message_id TEXT NOT NULL REFERENCES conversation_messages(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','completed','failed','cancelled','timed_out')),
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  error_code TEXT
);
CREATE INDEX IF NOT EXISTS agent_runs_conversation_idx ON agent_runs(conversation_id, started_at DESC);
CREATE TABLE IF NOT EXISTS conversation_events (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  sequence BIGSERIAL NOT NULL,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(conversation_id, sequence)
);
CREATE INDEX IF NOT EXISTS conversation_events_run_idx ON conversation_events(run_id, sequence);
CREATE TABLE IF NOT EXISTS conversation_audit (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  device_id TEXT NOT NULL,
  request_id TEXT,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  duration_ms BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS conversation_audit_account_idx ON conversation_audit(account_id, created_at DESC);
