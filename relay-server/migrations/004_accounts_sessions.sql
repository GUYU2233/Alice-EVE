-- Accounts, external identities, and rotating server sessions.
-- Raw access/refresh credentials are never persisted: *_token_hash columns
-- contain SHA-256 digests of opaque, CSPRNG-generated values.
CREATE TABLE IF NOT EXISTS accounts (
  id UUID PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  display_name TEXT,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identities (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (provider, subject)
);
CREATE INDEX IF NOT EXISTS identities_account_idx ON identities(account_id);

CREATE TABLE IF NOT EXISTS sessions (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  device_id TEXT,
  access_token_hash TEXT NOT NULL UNIQUE,
  access_expires_at TIMESTAMPTZ NOT NULL,
  refresh_token_hash TEXT NOT NULL UNIQUE,
  token_family_id UUID NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  family_revoked_at TIMESTAMPTZ,
  replaced_by_hash TEXT,
  rotated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS sessions_account_idx ON sessions(account_id);
CREATE INDEX IF NOT EXISTS sessions_family_idx ON sessions(token_family_id);
CREATE INDEX IF NOT EXISTS sessions_active_account_idx ON sessions(account_id) WHERE revoked_at IS NULL;
