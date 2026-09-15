-- Device-scoped reminder preferences and provider token registry.
-- Raw push tokens are intentionally not persisted: token_hash is SHA-256.
CREATE TABLE IF NOT EXISTS notification_preferences (
  device_id TEXT PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  etag TEXT NOT NULL,
  preferences JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS push_tokens (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  provider TEXT NOT NULL CHECK (provider IN ('fcm', 'apns', 'webpush')),
  platform TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  app_version TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ,
  UNIQUE(device_id, provider, token_hash)
);
CREATE INDEX IF NOT EXISTS push_tokens_active_idx ON push_tokens(device_id) WHERE revoked_at IS NULL;
