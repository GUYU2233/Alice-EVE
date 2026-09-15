// Package migrations provides a transactional, auditable PostgreSQL migration
// runner. The embedded migration set is deliberately limited to SQL files in
// this directory; callers cannot accidentally execute SQL from another path.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLFiles contains only the checked-in migrations adjacent to this package.
// It is exported so an application can construct a runner without relying on
// the process working directory or a host-mounted migrations directory.
//
//go:embed *.sql
var SQLFiles embed.FS

const schemaMigrationsTable = "schema_migrations"

var migrationNamePattern = regexp.MustCompile(`^([0-9]+)_([a-z0-9][a-z0-9_-]*)\.sql$`)

// Migration is one immutable, numbered SQL migration.
type Migration struct {
	Version  int64
	Name     string
	Filename string
	SQL      string
	Checksum string
}

// AppliedMigration is the audit record read from schema_migrations.
type AppliedMigration struct {
	Version   int64
	Name      string
	Checksum  string
	AppliedAt time.Time
}

// Plan describes the result of a read-only migration inspection.
type Plan struct {
	Migrations       []Migration
	Applied          []AppliedMigration
	Pending          []Migration
	SchemaTableFound bool
}

// ApplyResult records what Apply committed.
type ApplyResult struct {
	Applied []Migration
}

// Runner executes a validated, ordered migration set.
type Runner struct {
	migrations []Migration
}

// New returns a runner for the embedded relay-server/migrations SQL files.
func New() (*Runner, error) {
	return NewFromFS(SQLFiles)
}

// NewFromFS validates and loads only the root-level numbered SQL files in fsys.
// It is primarily useful for tests and for packaging a separately reviewed
// migration set. Subdirectories and non-SQL files are ignored, never executed.
func NewFromFS(fsys fs.FS) (*Runner, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int64]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Dir(entry.Name()) != "." {
			continue
		}
		match := migrationNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d: %s and %s", version, previous, entry.Name())
		}
		contents, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		sql := string(contents)
		if strings.TrimSpace(sql) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		digest := sha256.Sum256(contents)
		seen[version] = entry.Name()
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     match[2],
			Filename: entry.Name(),
			SQL:      sql,
			Checksum: hex.EncodeToString(digest[:]),
		})
	}
	if len(migrations) == 0 {
		return nil, errors.New("no numbered SQL migrations found")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return &Runner{migrations: migrations}, nil
}

// Migrations returns a defensive copy of the validated ordered migration set.
func (r *Runner) Migrations() []Migration {
	out := make([]Migration, len(r.migrations))
	copy(out, r.migrations)
	return out
}

// DryRun performs a read-only inspection. A fresh database without
// schema_migrations is reported as having every migration pending; the method
// never creates the table or executes migration SQL.
func (r *Runner) DryRun(ctx context.Context, pool *pgxpool.Pool) (Plan, error) {
	if pool == nil {
		return Plan{}, errors.New("migration dry-run requires a PostgreSQL pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return Plan{}, fmt.Errorf("begin read-only migration plan: %w", err)
	}
	defer tx.Rollback(ctx) // read-only transaction; rollback errors are irrelevant

	applied, found, err := readApplied(ctx, tx)
	if err != nil {
		return Plan{}, err
	}
	plan, err := r.plan(applied, found)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// Apply executes all pending migrations in ascending numeric order. The
// schema table is created inside the same transaction as the first migration.
// A transaction-scoped PostgreSQL advisory lock serializes concurrent runners.
// A checksum/name mismatch is treated as tampering and aborts the transaction.
func (r *Runner) Apply(ctx context.Context, pool *pgxpool.Pool) (ApplyResult, error) {
	if pool == nil {
		return ApplyResult{}, errors.New("migration apply requires a PostgreSQL pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ApplyResult{}, fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('alice_eve.schema_migrations', 0))`); err != nil {
		return ApplyResult{}, fmt.Errorf("acquire migration lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		checksum CHAR(64) NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return ApplyResult{}, fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, _, err := readApplied(ctx, tx)
	if err != nil {
		return ApplyResult{}, err
	}
	plan, err := r.plan(applied, true)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{Applied: make([]Migration, 0, len(plan.Pending))}
	for _, migration := range plan.Pending {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return ApplyResult{}, fmt.Errorf("apply migration %03d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version, name, checksum) VALUES($1, $2, $3)`, migration.Version, migration.Name, migration.Checksum); err != nil {
			return ApplyResult{}, fmt.Errorf("record migration %03d_%s: %w", migration.Version, migration.Name, err)
		}
		result.Applied = append(result.Applied, migration)
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, fmt.Errorf("commit migrations: %w", err)
	}
	return result, nil
}

func (r *Runner) plan(applied []AppliedMigration, found bool) (Plan, error) {
	byVersion := make(map[int64]AppliedMigration, len(applied))
	for _, item := range applied {
		if _, exists := byVersion[item.Version]; exists {
			return Plan{}, fmt.Errorf("schema_migrations contains duplicate version %d", item.Version)
		}
		byVersion[item.Version] = item
	}
	known := make(map[int64]Migration, len(r.migrations))
	for _, migration := range r.migrations {
		known[migration.Version] = migration
	}
	for _, item := range applied {
		migration, exists := known[item.Version]
		if !exists {
			return Plan{}, fmt.Errorf("database contains unknown migration version %d", item.Version)
		}
		if item.Name != migration.Name || !strings.EqualFold(strings.TrimSpace(item.Checksum), migration.Checksum) {
			return Plan{}, fmt.Errorf("migration %d checksum/name mismatch: database=%s/%s files=%s/%s", item.Version, item.Name, item.Checksum, migration.Name, migration.Checksum)
		}
	}
	pending := make([]Migration, 0, len(r.migrations))
	for _, migration := range r.migrations {
		if _, exists := byVersion[migration.Version]; !exists {
			pending = append(pending, migration)
		}
	}
	return Plan{
		Migrations:       r.Migrations(),
		Applied:          append([]AppliedMigration(nil), applied...),
		Pending:          pending,
		SchemaTableFound: found,
	}, nil
}

func readApplied(ctx context.Context, query pgxQuerier) ([]AppliedMigration, bool, error) {
	rows, err := query.Query(ctx, `SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	out := make([]AppliedMigration, 0)
	for rows.Next() {
		var item AppliedMigration
		if err := rows.Scan(&item.Version, &item.Name, &item.Checksum, &item.AppliedAt); err != nil {
			return nil, true, fmt.Errorf("scan schema_migrations: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return out, true, nil
}

type pgxQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}
