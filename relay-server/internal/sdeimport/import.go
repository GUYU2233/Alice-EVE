package sdeimport

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

type localized map[string]string

func (l localized) Best() string {
	if s := strings.TrimSpace(l["zh"]); s != "" {
		return s
	}
	return strings.TrimSpace(l["en"])
}

type record struct {
	Key             int64     `json:"_key"`
	Name            localized `json:"name"`
	Description     localized `json:"description"`
	Published       bool      `json:"published"`
	GroupID         int64     `json:"groupID"`
	MarketGroupID   *int64    `json:"marketGroupID"`
	ParentGroupID   *int64    `json:"parentGroupID"`
	RegionID        int64     `json:"regionID"`
	ConstellationID int64     `json:"constellationID"`
	Security        float64   `json:"securityStatus"`
	SolarSystemID   int64     `json:"solarSystemID"`
	Destination     struct {
		SolarSystemID int64 `json:"solarSystemID"`
	} `json:"destination"`
	PackagedVolume *float64 `json:"packagedVolume"`
	Volume         *float64 `json:"volume"`
}

type DB interface {
	Begin(context.Context) (pgx.Tx, error)
}
type Counts struct{ Types, MarketGroups, Regions, Constellations, Systems, Stargates int64 }

func Import(ctx context.Context, db DB, zipPath, version string) (Counts, error) {
	if strings.TrimSpace(version) == "" {
		return Counts{}, errors.New("version is required")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return Counts{}, err
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[filepath.Base(f.Name)] = f
	}
	required := []string{"types.jsonl", "marketGroups.jsonl", "mapRegions.jsonl", "mapConstellations.jsonl", "mapSolarSystems.jsonl", "mapStargates.jsonl"}
	for _, n := range required {
		if files[n] == nil {
			return Counts{}, fmt.Errorf("required zip entry %s missing", n)
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return Counts{}, err
	}
	defer tx.Rollback(ctx)
	var build int64
	err = tx.QueryRow(ctx, `INSERT INTO eve_sde_builds(version,source,state) VALUES($1,$2,'staging') RETURNING id`, version, filepath.Base(zipPath)).Scan(&build)
	if err != nil {
		return Counts{}, fmt.Errorf("create build: %w", err)
	}
	var c Counts
	specs := []struct {
		name, table string
		columns     []string
		row         func(record) []any
		count       *int64
	}{
		{"marketGroups.jsonl", "eve_market_groups", []string{"build_id", "market_group_id", "parent_group_id", "name", "description"}, func(r record) []any { return []any{build, r.Key, r.ParentGroupID, r.Name.Best(), r.Description.Best()} }, &c.MarketGroups},
		{"types.jsonl", "eve_types", []string{"build_id", "type_id", "group_id", "market_group_id", "name", "description", "published", "packaged_volume", "volume"}, func(r record) []any {
			return []any{build, r.Key, r.GroupID, r.MarketGroupID, r.Name.Best(), r.Description.Best(), r.Published, r.PackagedVolume, r.Volume}
		}, &c.Types},
		{"mapRegions.jsonl", "eve_regions", []string{"build_id", "region_id", "name"}, func(r record) []any { return []any{build, r.Key, r.Name.Best()} }, &c.Regions},
		{"mapConstellations.jsonl", "eve_constellations", []string{"build_id", "constellation_id", "region_id", "name"}, func(r record) []any { return []any{build, r.Key, r.RegionID, r.Name.Best()} }, &c.Constellations},
		{"mapSolarSystems.jsonl", "eve_systems", []string{"build_id", "system_id", "constellation_id", "name", "security_status"}, func(r record) []any { return []any{build, r.Key, r.ConstellationID, r.Name.Best(), r.Security} }, &c.Systems},
		{"mapStargates.jsonl", "eve_stargates", []string{"build_id", "stargate_id", "system_id", "destination_system_id"}, func(r record) []any { return []any{build, r.Key, r.SolarSystemID, r.Destination.SolarSystemID} }, &c.Stargates},
	}
	for _, s := range specs {
		rows, e := decode(files[s.name])
		if e != nil {
			return Counts{}, e
		}
		n, e := tx.CopyFrom(ctx, pgx.Identifier{s.table}, s.columns, pgx.CopyFromRows(mapRows(rows, s.row)))
		if e != nil {
			return Counts{}, fmt.Errorf("import %s: %w", s.name, e)
		}
		*s.count = n
	}
	if c.Types == 0 || c.MarketGroups == 0 || c.Regions == 0 || c.Constellations == 0 || c.Systems == 0 || c.Stargates == 0 {
		return Counts{}, fmt.Errorf("refusing incomplete build: %+v", c)
	}
	// Force all deferred graph constraints before changing the active pointer.
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		return Counts{}, fmt.Errorf("validate build: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE eve_sde_builds SET state='retired',activated_at=NULL WHERE state='active'`); err != nil {
		return Counts{}, err
	}
	if tag, e := tx.Exec(ctx, `UPDATE eve_sde_builds SET state='active',activated_at=now() WHERE id=$1 AND state='staging'`, build); e != nil || tag.RowsAffected() != 1 {
		if e == nil {
			e = errors.New("staging build disappeared")
		}
		return Counts{}, e
	}
	if err = tx.Commit(ctx); err != nil {
		return Counts{}, err
	}
	return c, nil
}
func decode(f *zip.File) ([]record, error) {
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return decodeJSONL(r)
}
func decodeJSONL(r io.Reader) ([]record, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64<<10), 16<<20)
	out := []record{}
	for s.Scan() {
		var x record
		if e := json.Unmarshal(s.Bytes(), &x); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, s.Err()
}
func mapRows(in []record, fn func(record) []any) [][]any {
	out := make([][]any, len(in))
	for i := range in {
		out[i] = fn(in[i])
	}
	return out
}
