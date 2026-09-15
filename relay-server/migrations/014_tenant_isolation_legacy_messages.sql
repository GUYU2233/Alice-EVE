-- Tenant isolation for legacy device/message storage.
-- This is intentionally a new forward migration; migrations 001-010 are immutable.
-- Rows created before account ownership existed are quarantined into the explicit
-- legacy tenant rather than being exposed to any authenticated account.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS account_id TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS account_id TEXT;

-- Preserve the old compatibility pairing namespace as a quarantine tenant.
UPDATE devices SET account_id = 'legacy' WHERE account_id IS NULL OR btrim(account_id) = '';
UPDATE messages m
SET account_id = COALESCE(NULLIF(btrim(d.account_id), ''), 'legacy')
FROM devices d
WHERE m.owner_device_id = d.id
  AND (m.account_id IS NULL OR btrim(m.account_id) = '');
UPDATE messages SET account_id = 'legacy'
WHERE account_id IS NULL OR btrim(account_id) = '';

ALTER TABLE devices ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE messages ALTER COLUMN account_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS devices_account_idx ON devices(account_id, revoked, created_at DESC);
CREATE INDEX IF NOT EXISTS messages_account_pending_cursor_idx
  ON messages(account_id, owner_device_id, acknowledged_at, cursor);
-- Message IDs are idempotent within a tenant. Keep the old primary key intact
-- for compatibility, while this unique index documents/enforces the tenant key.
CREATE UNIQUE INDEX IF NOT EXISTS messages_account_id_idx ON messages(account_id, id);
