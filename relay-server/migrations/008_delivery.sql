-- Per-account/device durable delivery cursor and outbox state. The unique key
-- prevents duplicate delivery when a publisher retries the same event.
CREATE TABLE IF NOT EXISTS delivery_events (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  recipient_device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  sender_device_id TEXT REFERENCES devices(id) ON DELETE SET NULL,
  message_id TEXT,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  cursor BIGSERIAL NOT NULL,
  status TEXT NOT NULL DEFAULT 'persisted'
    CHECK (status IN ('persisted', 'delivered', 'processed', 'failed')),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  delivered_at TIMESTAMPTZ,
  processed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  UNIQUE(account_id, recipient_device_id, sender_device_id, message_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS delivery_events_cursor_idx
  ON delivery_events(account_id, recipient_device_id, cursor);
CREATE INDEX IF NOT EXISTS delivery_events_replay_idx
  ON delivery_events(account_id, recipient_device_id, cursor);
CREATE INDEX IF NOT EXISTS delivery_events_pending_idx
  ON delivery_events(recipient_device_id, status, cursor);

CREATE TABLE IF NOT EXISTS device_delivery_cursors (
  account_id TEXT NOT NULL,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  last_acked_cursor BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(account_id, device_id)
);
