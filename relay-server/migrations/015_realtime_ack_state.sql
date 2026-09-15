-- Durable realtime ACK state is kept alongside delivery events. The existing
-- delivery_events and device_delivery_cursors tables are the event log and
-- stream cursor; this table records per-event processing disposition so ACKs
-- remain auditable and survive reconnects.
CREATE TABLE IF NOT EXISTS realtime_event_acks (
  account_id TEXT NOT NULL,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES delivery_events(id) ON DELETE CASCADE,
  cursor BIGINT NOT NULL CHECK (cursor >= 0),
  status TEXT NOT NULL CHECK (status IN ('received', 'processed', 'failed')),
  error TEXT,
  acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, device_id, event_id)
);
CREATE INDEX IF NOT EXISTS realtime_event_acks_cursor_idx
  ON realtime_event_acks(account_id, device_id, cursor);
