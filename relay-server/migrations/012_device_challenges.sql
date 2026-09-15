-- Durable one-shot device authorization challenges. Only a SHA-256 hash is stored.
CREATE TABLE IF NOT EXISTS device_auth_challenges (
  challenge_hash TEXT PRIMARY KEY CHECK (challenge_hash ~ '^[0-9a-f]{64}$'),
  account_id TEXT,
  provider TEXT,
  subject TEXT,
  display_name TEXT,
  device_name TEXT NOT NULL,
  device_type TEXT NOT NULL CHECK (device_type IN ('desktop','mobile')),
  public_key TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  consumed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS device_auth_challenges_expiry_idx ON device_auth_challenges(expires_at);
