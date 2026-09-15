-- Release metadata only; artifacts are served by object storage/CDN.
CREATE TABLE IF NOT EXISTS app_releases (
  id TEXT PRIMARY KEY,
  app TEXT NOT NULL,
  version TEXT NOT NULL,
  platform TEXT NOT NULL,
  arch TEXT NOT NULL,
  channel TEXT NOT NULL,
  min_supported_version TEXT,
  mandatory BOOLEAN NOT NULL DEFAULT FALSE,
  manifest JSONB NOT NULL,
  artifact_url TEXT NOT NULL,
  sha256 CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-fA-F]{64}$'),
  signature JSONB NOT NULL,
  published_at TIMESTAMPTZ NOT NULL,
  withdrawn_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(app, version, platform, arch, channel)
);
CREATE INDEX IF NOT EXISTS app_releases_lookup_idx
  ON app_releases(app, platform, arch, channel, published_at DESC);
CREATE TABLE IF NOT EXISTS update_acks (
  id BIGSERIAL PRIMARY KEY,
  release_id TEXT NOT NULL REFERENCES app_releases(id) ON DELETE CASCADE,
  account_id UUID NOT NULL,
  device_id TEXT NOT NULL,
  status TEXT NOT NULL,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS update_acks_device_idx ON update_acks(account_id, device_id, created_at DESC);
