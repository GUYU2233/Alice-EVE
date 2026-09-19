-- Versioned EVE static-data snapshots. Importers write a complete immutable build
-- while it is staging, validate it, then atomically promote that build to active.
CREATE TABLE eve_sde_builds (
  id BIGSERIAL PRIMARY KEY,
  version TEXT NOT NULL UNIQUE CHECK (length(btrim(version)) > 0),
  source TEXT NOT NULL CHECK (length(btrim(source)) > 0),
  state TEXT NOT NULL DEFAULT 'staging' CHECK (state IN ('staging','active','retired','failed')),
  imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_at TIMESTAMPTZ,
  CHECK ((state = 'active') = (activated_at IS NOT NULL))
);
CREATE UNIQUE INDEX eve_sde_one_active_idx ON eve_sde_builds ((state)) WHERE state='active';

CREATE TABLE eve_market_groups (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  market_group_id BIGINT NOT NULL CHECK (market_group_id > 0),
  parent_group_id BIGINT,
  name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
  description TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (build_id, market_group_id),
  FOREIGN KEY (build_id, parent_group_id) REFERENCES eve_market_groups(build_id, market_group_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX eve_market_groups_parent_idx ON eve_market_groups(build_id,parent_group_id);

CREATE TABLE eve_types (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  type_id BIGINT NOT NULL CHECK (type_id > 0),
  group_id BIGINT NOT NULL CHECK (group_id > 0),
  market_group_id BIGINT,
  name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
  description TEXT NOT NULL DEFAULT '',
  published BOOLEAN NOT NULL DEFAULT false,
  packaged_volume NUMERIC,
  volume NUMERIC,
  PRIMARY KEY (build_id, type_id),
  FOREIGN KEY (build_id, market_group_id) REFERENCES eve_market_groups(build_id, market_group_id) DEFERRABLE INITIALLY DEFERRED,
  CHECK (packaged_volume IS NULL OR packaged_volume >= 0),
  CHECK (volume IS NULL OR volume >= 0)
);
CREATE INDEX eve_types_market_group_idx ON eve_types(build_id,market_group_id);

CREATE TABLE eve_regions (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  region_id BIGINT NOT NULL CHECK (region_id > 0), name TEXT NOT NULL CHECK (length(btrim(name)) > 0),
  PRIMARY KEY(build_id,region_id)
);
CREATE TABLE eve_constellations (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  constellation_id BIGINT NOT NULL CHECK (constellation_id > 0), region_id BIGINT NOT NULL,
  name TEXT NOT NULL CHECK (length(btrim(name)) > 0), PRIMARY KEY(build_id,constellation_id),
  FOREIGN KEY(build_id,region_id) REFERENCES eve_regions(build_id,region_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE eve_systems (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  system_id BIGINT NOT NULL CHECK (system_id > 0), constellation_id BIGINT NOT NULL,
  name TEXT NOT NULL CHECK (length(btrim(name)) > 0), security_status DOUBLE PRECISION NOT NULL,
  PRIMARY KEY(build_id,system_id),
  FOREIGN KEY(build_id,constellation_id) REFERENCES eve_constellations(build_id,constellation_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX eve_systems_constellation_idx ON eve_systems(build_id,constellation_id);
CREATE TABLE eve_stargates (
  build_id BIGINT NOT NULL REFERENCES eve_sde_builds(id) ON DELETE CASCADE,
  stargate_id BIGINT NOT NULL CHECK (stargate_id > 0), system_id BIGINT NOT NULL, destination_system_id BIGINT NOT NULL,
  PRIMARY KEY(build_id,stargate_id),
  FOREIGN KEY(build_id,system_id) REFERENCES eve_systems(build_id,system_id) DEFERRABLE INITIALLY DEFERRED,
  FOREIGN KEY(build_id,destination_system_id) REFERENCES eve_systems(build_id,system_id) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX eve_stargates_route_idx ON eve_stargates(build_id,system_id,destination_system_id);
