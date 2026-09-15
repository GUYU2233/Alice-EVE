-- Durable monotonic cursor for per-device incremental synchronization.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS cursor BIGSERIAL;
CREATE UNIQUE INDEX IF NOT EXISTS messages_cursor_idx ON messages(cursor);
CREATE INDEX IF NOT EXISTS messages_owner_cursor_pending_idx ON messages(owner_device_id, acknowledged_at, cursor);
