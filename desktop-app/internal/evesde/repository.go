package evesde

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("evesde: not found")

type Repository struct{ db *sql.DB }

type Named struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// SearchResult is the common named-record view used by compatibility clients.
type SearchResult struct {
	ID   int64
	Name string
	Kind string
}
type Type struct {
	ID, GroupID       int64
	Name, Description string
	Published         bool
	MarketGroupID     *int64
	PackagedVolume    float64
	Volume            float64
	Capacity          float64
}
type System struct {
	ID, ConstellationID int64
	Name                string
	Security            float64
}
type Stargate struct{ ID, SystemID, DestinationSystemID int64 }
type Blueprint struct {
	BlueprintTypeID int64
	ProductTypeID   *int64
	ProductionTime  int64
	Materials       []Material
}
type Material struct{ TypeID, Quantity int64 }
type StoredManifest struct {
	SchemaVersion             int
	Version, Source, Checksum string
	ImportedAt                time.Time
}

func Open(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	r := &Repository{db: db}
	var version int
	if err = db.QueryRow(`SELECT schema_version FROM manifest WHERE singleton=1`).Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version != SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("evesde: unsupported schema version %d", version)
	}
	return r, nil
}
func (r *Repository) Close() error { return r.db.Close() }

// NamedCount returns the number of records exposed by SearchNamed.
func (r *Repository) NamedCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM categories)+(SELECT count(*) FROM groups)+
		(SELECT count(*) FROM types)+(SELECT count(*) FROM regions)+
		(SELECT count(*) FROM constellations)+(SELECT count(*) FROM systems)+
		(SELECT count(*) FROM market_groups)`).Scan(&count)
	return count, err
}

func (r *Repository) Manifest(ctx context.Context) (StoredManifest, error) {
	var m StoredManifest
	var imported string
	err := r.db.QueryRowContext(ctx, `SELECT schema_version,version,source,checksum,imported_at FROM manifest WHERE singleton=1`).Scan(&m.SchemaVersion, &m.Version, &m.Source, &m.Checksum, &imported)
	if err == nil {
		m.ImportedAt, err = time.Parse(time.RFC3339Nano, imported)
	}
	return m, err
}
func (r *Repository) TypeByID(ctx context.Context, id int64) (Type, error) {
	return r.typeOne(ctx, `SELECT id,group_id,name,description,published,market_group_id,packaged_volume,volume,capacity FROM types WHERE id=?`, id)
}
func (r *Repository) TypeByName(ctx context.Context, name string) (Type, error) {
	return r.typeOne(ctx, `SELECT id,group_id,name,description,published,market_group_id,packaged_volume,volume,capacity FROM types WHERE name=? COLLATE NOCASE`, name)
}
func (r *Repository) typeOne(ctx context.Context, q string, arg any) (Type, error) {
	var x Type
	var p int
	err := r.db.QueryRowContext(ctx, q, arg).Scan(&x.ID, &x.GroupID, &x.Name, &x.Description, &p, &x.MarketGroupID, &x.PackagedVolume, &x.Volume, &x.Capacity)
	x.Published = p != 0
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}
func (r *Repository) SystemByID(ctx context.Context, id int64) (System, error) {
	return r.systemOne(ctx, `SELECT id,constellation_id,name,security FROM systems WHERE id=?`, id)
}
func (r *Repository) SystemByName(ctx context.Context, name string) (System, error) {
	return r.systemOne(ctx, `SELECT id,constellation_id,name,security FROM systems WHERE name=? COLLATE NOCASE`, name)
}
func (r *Repository) systemOne(ctx context.Context, q string, arg any) (System, error) {
	var x System
	err := r.db.QueryRowContext(ctx, q, arg).Scan(&x.ID, &x.ConstellationID, &x.Name, &x.Security)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}

func (r *Repository) SearchTypes(ctx context.Context, query string, limit int) ([]Type, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []Type{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,group_id,name,description,published,market_group_id,packaged_volume,volume,capacity FROM types WHERE name LIKE ? ESCAPE '\' COLLATE NOCASE ORDER BY CASE WHEN name=? COLLATE NOCASE THEN 0 ELSE 1 END,name LIMIT ?`, "%"+escapeLike(query)+"%", query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Type{}
	for rows.Next() {
		var x Type
		var p int
		if err = rows.Scan(&x.ID, &x.GroupID, &x.Name, &x.Description, &p, &x.MarketGroupID, &x.PackagedVolume, &x.Volume, &x.Capacity); err != nil {
			return nil, err
		}
		x.Published = p != 0
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) SearchSystems(ctx context.Context, query string, limit int) ([]System, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []System{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,constellation_id,name,security FROM systems WHERE name LIKE ? ESCAPE '\' COLLATE NOCASE ORDER BY CASE WHEN name=? COLLATE NOCASE THEN 0 ELSE 1 END,name LIMIT ?`, "%"+escapeLike(query)+"%", query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []System{}
	for rows.Next() {
		var x System
		if err = rows.Scan(&x.ID, &x.ConstellationID, &x.Name, &x.Security); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SearchNamed searches all named entities while preserving deterministic ordering
// and a single combined limit for compatibility with the legacy SDE index.
func (r *Repository) SearchNamed(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []SearchResult{}, nil
	}
	like := "%" + escapeLike(query) + "%"
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,name,kind FROM (
			SELECT id,name,'category' AS kind FROM categories
			UNION ALL SELECT id,name,'group' FROM groups
			UNION ALL SELECT id,name,'type' FROM types
			UNION ALL SELECT id,name,'region' FROM regions
			UNION ALL SELECT id,name,'constellation' FROM constellations
			UNION ALL SELECT id,name,'system' FROM systems
			UNION ALL SELECT id,name,'market_group' FROM market_groups
		) WHERE name LIKE ? ESCAPE '\' COLLATE NOCASE OR CAST(id AS TEXT)=?
		ORDER BY CASE WHEN name=? COLLATE NOCASE OR CAST(id AS TEXT)=? THEN 0 ELSE 1 END,
		         name COLLATE NOCASE,id,kind LIMIT ?`, like, query, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SearchResult, 0, limit)
	for rows.Next() {
		var x SearchResult
		if err = rows.Scan(&x.ID, &x.Name, &x.Kind); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (r *Repository) CategoryByID(ctx context.Context, id int64) (Named, error) {
	return r.named(ctx, "categories", "id", id)
}
func (r *Repository) CategoryByName(ctx context.Context, n string) (Named, error) {
	return r.named(ctx, "categories", "name", n)
}
func (r *Repository) GroupByID(ctx context.Context, id int64) (Named, error) {
	return r.named(ctx, "groups", "id", id)
}
func (r *Repository) GroupByName(ctx context.Context, n string) (Named, error) {
	return r.named(ctx, "groups", "name", n)
}
func (r *Repository) RegionForSystem(ctx context.Context, systemID int64) (Named, error) {
	var x Named
	err := r.db.QueryRowContext(ctx, `SELECT r.id,r.name FROM systems s JOIN constellations c ON c.id=s.constellation_id JOIN regions r ON r.id=c.region_id WHERE s.id=?`, systemID).Scan(&x.ID, &x.Name)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}
func (r *Repository) Regions(ctx context.Context) ([]Named, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name FROM regions ORDER BY name COLLATE NOCASE,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Named{}
	for rows.Next() {
		var x Named
		if err = rows.Scan(&x.ID, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) RegionByID(ctx context.Context, id int64) (Named, error) {
	return r.named(ctx, "regions", "id", id)
}
func (r *Repository) RegionByName(ctx context.Context, n string) (Named, error) {
	return r.named(ctx, "regions", "name", n)
}
func (r *Repository) ConstellationByID(ctx context.Context, id int64) (Named, error) {
	return r.named(ctx, "constellations", "id", id)
}
func (r *Repository) ConstellationByName(ctx context.Context, n string) (Named, error) {
	return r.named(ctx, "constellations", "name", n)
}
func (r *Repository) MarketGroupByID(ctx context.Context, id int64) (Named, error) {
	return r.named(ctx, "market_groups", "id", id)
}
func (r *Repository) MarketGroupByName(ctx context.Context, n string) (Named, error) {
	return r.named(ctx, "market_groups", "name", n)
}
func (r *Repository) named(ctx context.Context, table, col string, arg any) (Named, error) {
	var x Named
	err := r.db.QueryRowContext(ctx, `SELECT id,name FROM `+table+` WHERE `+col+`=? COLLATE NOCASE`, arg).Scan(&x.ID, &x.Name)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}

func (r *Repository) StargatesFromSystem(ctx context.Context, systemID int64) ([]Stargate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,system_id,destination_system_id FROM stargates WHERE system_id=? ORDER BY id`, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stargate{}
	for rows.Next() {
		var x Stargate
		if err = rows.Scan(&x.ID, &x.SystemID, &x.DestinationSystemID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) BlueprintByTypeID(ctx context.Context, id int64) (Blueprint, error) {
	var x Blueprint
	err := r.db.QueryRowContext(ctx, `SELECT blueprint_type_id,product_type_id,production_time FROM blueprints WHERE blueprint_type_id=?`, id).Scan(&x.BlueprintTypeID, &x.ProductTypeID, &x.ProductionTime)
	if errors.Is(err, sql.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT material_type_id,quantity FROM blueprint_materials WHERE blueprint_type_id=? ORDER BY material_type_id`, id)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Material
		if err = rows.Scan(&m.TypeID, &m.Quantity); err != nil {
			return x, err
		}
		x.Materials = append(x.Materials, m)
	}
	return x, rows.Err()
}
