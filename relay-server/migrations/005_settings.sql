-- Settings documents are tenant-scoped, versioned JSON objects.
-- account_id intentionally has no FK until the accounts migration is deployed;
-- deployments may add the FK after existing account data is backfilled.
CREATE TABLE IF NOT EXISTS settings_documents (
  account_id UUID NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('account', 'device', 'profile')),
  scope_id TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  etag TEXT NOT NULL,
  document JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_by_device_id TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (account_id, scope, scope_id),
  CHECK ((scope = 'account' AND scope_id = '') OR (scope <> 'account' AND scope_id <> ''))
);

CREATE TABLE IF NOT EXISTS settings_changes (
  id BIGSERIAL PRIMARY KEY,
  account_id UUID NOT NULL,
  scope TEXT NOT NULL,
  scope_id TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL CHECK (version > 0),
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (scope IN ('account', 'device', 'profile'))
);
CREATE INDEX IF NOT EXISTS settings_changes_account_cursor_idx
  ON settings_changes(account_id, id);
CREATE INDEX IF NOT EXISTS settings_changes_scope_idx
  ON settings_changes(account_id, scope, scope_id, version);
