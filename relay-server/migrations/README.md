# Database migrations

`migrations` exposes a small PostgreSQL runner for the Alice-EVE Server. It embeds only root-level files matching the reviewed filename contract:

```text
<positive-decimal-version>_<lowercase-alphanumeric-name-with-_or->.sql
```

Files are ordered by their numeric prefix. Duplicate versions, empty files, unknown applied versions, renamed migrations, and changed checksums fail closed. Each pending migration and its SHA-256 checksum is recorded in the PostgreSQL `schema_migrations` table.

## Use from an operator/app package

The `cmd/relay-server` entrypoint runs the embedded runner before opening the HTTP listener when `DATABASE_URL` is set. Configure `MIGRATIONS_MODE=apply` (the default; aliases `auto/true/1`), `dry-run` (aliases `dryrun/plan`), or `off` (aliases `disabled/false/0`); invalid values fail closed. `off` is intended only when an equivalent deployment step runs the runner separately.

For library use:

```go
runner, err := migrations.New()
if err != nil { return err }

// Read-only inspection: does not create tables or run SQL.
plan, err := runner.DryRun(ctx, pool)

// Apply all pending files in one transaction, serialized with a PostgreSQL
// advisory transaction lock.
result, err := runner.Apply(ctx, pool)
```

`Apply` requires a `*pgxpool.Pool`, applies migrations in ascending order, creates `schema_migrations` inside the same transaction, and rolls back the complete batch if any SQL or audit insert fails. `DryRun` uses a read-only transaction and returns the applied/pending plan without modifying the database.

Use a dedicated migration database role for deployment. Keep DSNs and credentials outside the repository, and review the SQL files as code. When `DATABASE_URL` is configured, the service entrypoint invokes this runner before opening the listener. Use `off` only when an equivalent external deployment step has already run the same migrations.
