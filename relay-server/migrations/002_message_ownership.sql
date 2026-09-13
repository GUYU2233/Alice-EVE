-- Backward-compatible backlog ownership migration.
-- Existing rows remain visible until an explicit owner is assigned; new API writes should set owner_device_id.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS owner_device_id TEXT REFERENCES devices(id) ON DELETE CASCADE;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS messages_owner_pending_idx ON messages(owner_device_id, acknowledged_at, id);
