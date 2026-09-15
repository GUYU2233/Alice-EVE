-- Per-target cursor allocation and durable acknowledgement state.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS account_id TEXT;
CREATE INDEX IF NOT EXISTS devices_account_idx ON devices(account_id);
ALTER TABLE device_delivery_cursors ADD COLUMN IF NOT EXISTS next_cursor BIGINT NOT NULL DEFAULT 0;
ALTER TABLE device_delivery_cursors ADD COLUMN IF NOT EXISTS first_available_cursor BIGINT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS delivery_events_status_retry_idx ON delivery_events(recipient_device_id,status,created_at);
