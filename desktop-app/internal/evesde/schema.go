// Package evesde provides a small, versioned SQLite representation of normalized EVE SDE data.
package evesde

const SchemaVersion = 2

const schemaSQL = `
PRAGMA foreign_keys = ON;
CREATE TABLE manifest (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  schema_version INTEGER NOT NULL,
  version TEXT NOT NULL,
  source TEXT NOT NULL,
  checksum TEXT NOT NULL,
  imported_at TEXT NOT NULL
);
CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT NOT NULL COLLATE NOCASE, published INTEGER NOT NULL DEFAULT 1);
CREATE INDEX categories_name ON categories(name);
CREATE TABLE groups (id INTEGER PRIMARY KEY, category_id INTEGER NOT NULL REFERENCES categories(id), name TEXT NOT NULL COLLATE NOCASE, published INTEGER NOT NULL DEFAULT 1);
CREATE INDEX groups_category ON groups(category_id); CREATE INDEX groups_name ON groups(name);
CREATE TABLE types (id INTEGER PRIMARY KEY, group_id INTEGER NOT NULL REFERENCES groups(id), name TEXT NOT NULL COLLATE NOCASE, description TEXT NOT NULL DEFAULT '', published INTEGER NOT NULL DEFAULT 1, market_group_id INTEGER REFERENCES market_groups(id), packaged_volume REAL NOT NULL DEFAULT 0, volume REAL NOT NULL DEFAULT 0, capacity REAL NOT NULL DEFAULT 0);
CREATE INDEX types_name ON types(name); CREATE INDEX types_group ON types(group_id);
CREATE TABLE regions (id INTEGER PRIMARY KEY, name TEXT NOT NULL COLLATE NOCASE); CREATE UNIQUE INDEX regions_name ON regions(name);
CREATE TABLE constellations (id INTEGER PRIMARY KEY, region_id INTEGER NOT NULL REFERENCES regions(id), name TEXT NOT NULL COLLATE NOCASE); CREATE UNIQUE INDEX constellations_name ON constellations(name); CREATE INDEX constellations_region ON constellations(region_id);
CREATE TABLE systems (id INTEGER PRIMARY KEY, constellation_id INTEGER NOT NULL REFERENCES constellations(id), name TEXT NOT NULL COLLATE NOCASE, security REAL NOT NULL); CREATE UNIQUE INDEX systems_name ON systems(name); CREATE INDEX systems_constellation ON systems(constellation_id);
CREATE TABLE stargates (id INTEGER PRIMARY KEY, system_id INTEGER NOT NULL REFERENCES systems(id), destination_system_id INTEGER NOT NULL REFERENCES systems(id)); CREATE INDEX stargates_system ON stargates(system_id);
CREATE TABLE market_groups (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES market_groups(id), name TEXT NOT NULL COLLATE NOCASE, description TEXT NOT NULL DEFAULT ''); CREATE INDEX market_groups_name ON market_groups(name);
CREATE TABLE blueprints (blueprint_type_id INTEGER PRIMARY KEY, product_type_id INTEGER, production_time INTEGER NOT NULL DEFAULT 0);
CREATE TABLE blueprint_materials (blueprint_type_id INTEGER NOT NULL REFERENCES blueprints(blueprint_type_id) ON DELETE CASCADE, material_type_id INTEGER NOT NULL, quantity INTEGER NOT NULL CHECK(quantity > 0), PRIMARY KEY(blueprint_type_id, material_type_id));
`
