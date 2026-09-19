package evesde

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrResourceLimit = errors.New("evesde: import resource limit exceeded")
	ErrInvalidRecord = errors.New("evesde: invalid record")
)

type Manifest struct {
	Version  string
	Source   string
	Checksum string
}

type ImportOptions struct {
	MaxBytes     int64
	MaxRecords   int
	MaxLineBytes int
}

type record struct {
	Kind                string  `json:"kind"`
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	Published           *bool   `json:"published,omitempty"`
	CategoryID          int64   `json:"category_id,omitempty"`
	GroupID             int64   `json:"group_id,omitempty"`
	MarketGroupID       *int64  `json:"market_group_id,omitempty"`
	Description         string  `json:"description,omitempty"`
	RegionID            int64   `json:"region_id,omitempty"`
	ConstellationID     int64   `json:"constellation_id,omitempty"`
	Security            float64 `json:"security,omitempty"`
	SystemID            int64   `json:"system_id,omitempty"`
	DestinationSystemID int64   `json:"destination_system_id,omitempty"`
	ParentID            *int64  `json:"parent_id,omitempty"`
	BlueprintTypeID     int64   `json:"blueprint_type_id,omitempty"`
	ProductTypeID       *int64  `json:"product_type_id,omitempty"`
	ProductionTime      int64   `json:"production_time,omitempty"`
	MaterialTypeID      int64   `json:"material_type_id,omitempty"`
	Quantity            int64   `json:"quantity,omitempty"`
	PackagedVolume      float64 `json:"packaged_volume,omitempty"`
	Volume              float64 `json:"volume,omitempty"`
	Capacity            float64 `json:"capacity,omitempty"`
}

// Import builds a complete database beside targetPath and switches it into place only after validation.
// The input is normalized JSONL, with one object per category/group/type/region/constellation/system/stargate/market_group/blueprint/material.
func Import(ctx context.Context, targetPath string, src io.Reader, manifest Manifest, opts ImportOptions) error {
	if strings.TrimSpace(targetPath) == "" || src == nil || strings.TrimSpace(manifest.Version) == "" || strings.TrimSpace(manifest.Source) == "" {
		return fmt.Errorf("%w: target, version and source are required", ErrInvalidRecord)
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 512 << 20
	}
	if opts.MaxRecords <= 0 {
		opts.MaxRecords = 2_000_000
	}
	if opts.MaxLineBytes <= 0 {
		opts.MaxLineBytes = 4 << 20
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
		return err
	}

	h := sha256.New()
	limited := &io.LimitedReader{R: src, N: opts.MaxBytes + 1}
	scanner := bufio.NewScanner(io.TeeReader(limited, h))
	scanner.Buffer(make([]byte, 64<<10), opts.MaxLineBytes)
	records := make([]record, 0, min(opts.MaxRecords, 4096))
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if limited.N <= 0 {
			return ErrResourceLimit
		}
		if len(records) >= opts.MaxRecords {
			return ErrResourceLimit
		}
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var r record
		dec := json.NewDecoder(strings.NewReader(string(line)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return fmt.Errorf("%w at line %d: %v", ErrInvalidRecord, len(records)+1, err)
		}
		if err := validateRecord(r); err != nil {
			return fmt.Errorf("line %d: %w", len(records)+1, err)
		}
		records = append(records, r)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return ErrResourceLimit
		}
		return err
	}
	if limited.N <= 0 {
		return ErrResourceLimit
	}
	checksum := hex.EncodeToString(h.Sum(nil))
	if manifest.Checksum != "" && !strings.EqualFold(manifest.Checksum, checksum) {
		return fmt.Errorf("checksum: expected %s, got %s", manifest.Checksum, checksum)
	}
	manifest.Checksum = checksum

	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".evesde-*.sqlite")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	if err = buildDB(ctx, tmpPath, records, manifest); err != nil {
		return err
	}
	return replaceFile(tmpPath, targetPath)
}

func validateRecord(r record) error {
	if r.ID < 0 && r.Kind != "blueprint" && r.Kind != "material" {
		return fmt.Errorf("%w: non-negative id required", ErrInvalidRecord)
	}
	nameRequired := r.Kind == "category" || r.Kind == "group" || r.Kind == "type" || r.Kind == "region" || r.Kind == "constellation" || r.Kind == "system" || r.Kind == "market_group"
	if nameRequired && strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("%w: name required", ErrInvalidRecord)
	}
	switch r.Kind {
	case "category", "region", "market_group":
	case "group":
		if r.CategoryID < 0 {
			return fmt.Errorf("%w: invalid category_id", ErrInvalidRecord)
		}
	case "type":
		if r.GroupID < 0 {
			return fmt.Errorf("%w: invalid group_id", ErrInvalidRecord)
		}
	case "constellation":
		if r.RegionID <= 0 {
			return fmt.Errorf("%w: region_id required", ErrInvalidRecord)
		}
	case "system":
		if r.ConstellationID <= 0 {
			return fmt.Errorf("%w: constellation_id required", ErrInvalidRecord)
		}
	case "stargate":
		if r.SystemID <= 0 || r.DestinationSystemID <= 0 {
			return fmt.Errorf("%w: system ids required", ErrInvalidRecord)
		}
	case "blueprint":
		if r.BlueprintTypeID <= 0 || r.ProductionTime < 0 {
			return fmt.Errorf("%w: invalid blueprint", ErrInvalidRecord)
		}
	case "material":
		if r.BlueprintTypeID <= 0 || r.MaterialTypeID <= 0 || r.Quantity <= 0 {
			return fmt.Errorf("%w: invalid material", ErrInvalidRecord)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidRecord, r.Kind)
	}
	return nil
}

func buildDB(ctx context.Context, path string, records []record, m Manifest) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	order := []string{"category", "region", "market_group", "group", "type", "constellation", "system", "stargate", "blueprint", "material"}
	for _, kind := range order {
		for _, r := range records {
			if r.Kind == kind {
				if err = insertRecord(ctx, tx, r); err != nil {
					return fmt.Errorf("insert %s %d: %w", kind, r.ID, err)
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO manifest(singleton,schema_version,version,source,checksum,imported_at) VALUES(1,?,?,?,?,?)`, SchemaVersion, m.Version, m.Source, m.Checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var violations int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil {
		return err
	}
	if violations != 0 {
		return fmt.Errorf("%w: %d foreign-key violations", ErrInvalidRecord, violations)
	}
	_, err = db.ExecContext(ctx, `PRAGMA optimize`)
	return err
}

func published(p *bool) int {
	if p == nil || *p {
		return 1
	}
	return 0
}
func insertRecord(ctx context.Context, tx *sql.Tx, r record) error {
	var q string
	var a []any
	switch r.Kind {
	case "category":
		q = `INSERT INTO categories(id,name,published) VALUES(?,?,?)`
		a = []any{r.ID, r.Name, published(r.Published)}
	case "group":
		q = `INSERT INTO groups(id,category_id,name,published) VALUES(?,?,?,?)`
		a = []any{r.ID, r.CategoryID, r.Name, published(r.Published)}
	case "type":
		q = `INSERT INTO types(id,group_id,name,description,published,market_group_id,packaged_volume,volume,capacity) VALUES(?,?,?,?,?,?,?,?,?)`
		a = []any{r.ID, r.GroupID, r.Name, r.Description, published(r.Published), r.MarketGroupID, r.PackagedVolume, r.Volume, r.Capacity}
	case "region":
		q = `INSERT INTO regions(id,name) VALUES(?,?)`
		a = []any{r.ID, r.Name}
	case "constellation":
		q = `INSERT INTO constellations(id,region_id,name) VALUES(?,?,?)`
		a = []any{r.ID, r.RegionID, r.Name}
	case "system":
		q = `INSERT INTO systems(id,constellation_id,name,security) VALUES(?,?,?,?)`
		a = []any{r.ID, r.ConstellationID, r.Name, r.Security}
	case "stargate":
		q = `INSERT INTO stargates(id,system_id,destination_system_id) VALUES(?,?,?)`
		a = []any{r.ID, r.SystemID, r.DestinationSystemID}
	case "market_group":
		q = `INSERT INTO market_groups(id,parent_id,name,description) VALUES(?,?,?,?)`
		a = []any{r.ID, r.ParentID, r.Name, r.Description}
	case "blueprint":
		q = `INSERT INTO blueprints(blueprint_type_id,product_type_id,production_time) VALUES(?,?,?)`
		a = []any{r.BlueprintTypeID, r.ProductTypeID, r.ProductionTime}
	case "material":
		q = `INSERT INTO blueprint_materials(blueprint_type_id,material_type_id,quantity) VALUES(?,?,?)`
		a = []any{r.BlueprintTypeID, r.MaterialTypeID, r.Quantity}
	}
	_, err := tx.ExecContext(ctx, q, a...)
	return err
}

func replaceFile(tmp, target string) error {
	backup := target + ".old"
	_ = os.Remove(backup)
	if _, err := os.Stat(target); err == nil {
		if err = os.Rename(target, backup); err != nil {
			return err
		}
		if err = os.Rename(tmp, target); err != nil {
			_ = os.Rename(backup, target)
			return err
		}
		_ = os.Remove(backup)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, target)
}
