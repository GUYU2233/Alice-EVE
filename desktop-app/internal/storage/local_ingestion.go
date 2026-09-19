package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// LogSource identifies a kind of local log input and its configured root path.
type LogSource struct {
	SourceKind string
	RootPath   string
	UpdatedMS  int64
}

// LogFile is the latest observed metadata for a file belonging to a source.
type LogFile struct {
	SourceKind   string
	Path         string
	FileIdentity string
	Generation   int64
	Size         int64
	ModifiedMS   int64
	UpdatedMS    int64
}

// LogCursor is the durable read position for one source path.
type LogCursor struct {
	SourceKind      string
	Path            string
	FileIdentity    string
	Generation      int64
	CommittedOffset int64
	Encoding        string
	Size            int64
	ModifiedMS      int64
	UpdatedMS       int64
}

// NormalizedLocalEvent is a source-independent event parsed from a local log.
type NormalizedLocalEvent struct {
	ID           string
	SourceKind   string
	SourcePath   string
	FileIdentity string
	RecordStart  int64
	RecordEnd    int64
	EventType    string
	ObservedMS   int64
	PayloadJSON  string
	CreatedMS    int64
}

func ensureLocalIngestionSchema(ctx context.Context, s Store) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS log_sources (
			source_kind TEXT PRIMARY KEY,
			root_path TEXT NOT NULL,
			updated_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS log_files (
			source_kind TEXT NOT NULL,
			path TEXT NOT NULL,
			file_identity TEXT NOT NULL,
			generation INTEGER NOT NULL,
			size INTEGER NOT NULL,
			modified_ms INTEGER NOT NULL,
			updated_ms INTEGER NOT NULL,
			PRIMARY KEY (source_kind, path)
		)`,
		`CREATE TABLE IF NOT EXISTS log_cursors (
			source_kind TEXT NOT NULL,
			path TEXT NOT NULL,
			file_identity TEXT NOT NULL,
			generation INTEGER NOT NULL,
			committed_offset INTEGER NOT NULL,
			encoding TEXT NOT NULL,
			size INTEGER NOT NULL,
			modified_ms INTEGER NOT NULL,
			updated_ms INTEGER NOT NULL,
			PRIMARY KEY (source_kind, path)
		)`,
		`CREATE TABLE IF NOT EXISTS normalized_local_events (
			id TEXT PRIMARY KEY,
			source_kind TEXT NOT NULL,
			source_path TEXT NOT NULL,
			file_identity TEXT NOT NULL,
			record_start INTEGER NOT NULL,
			record_end INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			observed_ms INTEGER NOT NULL,
			payload_json TEXT NOT NULL,
			created_ms INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_normalized_local_events_source
			ON normalized_local_events(source_kind, source_path, record_start)`,
	}
	for _, statement := range statements {
		if _, err := s.Exec(ctx, statement); err != nil {
			return fmt.Errorf("create local ingestion schema: %w", err)
		}
	}
	return nil
}

func UpsertLogSource(ctx context.Context, s Store, source LogSource) error {
	_, err := s.Exec(ctx, `INSERT INTO log_sources(source_kind,root_path,updated_ms)
		VALUES(?,?,?)
		ON CONFLICT(source_kind) DO UPDATE SET root_path=excluded.root_path,updated_ms=excluded.updated_ms`,
		source.SourceKind, source.RootPath, source.UpdatedMS)
	return err
}

func UpsertLogFile(ctx context.Context, s Store, file LogFile) error {
	_, err := s.Exec(ctx, `INSERT INTO log_files(source_kind,path,file_identity,generation,size,modified_ms,updated_ms)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(source_kind,path) DO UPDATE SET
			file_identity=excluded.file_identity,generation=excluded.generation,size=excluded.size,
			modified_ms=excluded.modified_ms,updated_ms=excluded.updated_ms`,
		file.SourceKind, file.Path, file.FileIdentity, file.Generation, file.Size, file.ModifiedMS, file.UpdatedMS)
	return err
}

func LoadLogCursor(ctx context.Context, s Store, sourceKind, path string) (LogCursor, bool, error) {
	rows, err := s.Query(ctx, `SELECT source_kind,path,file_identity,generation,committed_offset,encoding,size,modified_ms,updated_ms
		FROM log_cursors WHERE source_kind=? AND path=?`, sourceKind, path)
	if err != nil {
		return LogCursor{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return LogCursor{}, false, rows.Err()
	}
	var cursor LogCursor
	if err = rows.Scan(&cursor.SourceKind, &cursor.Path, &cursor.FileIdentity, &cursor.Generation,
		&cursor.CommittedOffset, &cursor.Encoding, &cursor.Size, &cursor.ModifiedMS, &cursor.UpdatedMS); err != nil {
		return LogCursor{}, false, err
	}
	return cursor, true, rows.Err()
}

// CommitNormalizedEvents atomically inserts new events and advances the cursor.
// Event IDs are stable uniqueness keys; replaying an event is intentionally ignored.
func ListNormalizedLocalEvents(ctx context.Context, s Store, sourceKind string, limit int) ([]NormalizedLocalEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.Query(ctx, `SELECT id,source_kind,source_path,file_identity,record_start,record_end,event_type,observed_ms,payload_json,created_ms
		FROM normalized_local_events WHERE (?='' OR source_kind=?) ORDER BY observed_ms DESC, created_ms DESC LIMIT ?`, sourceKind, sourceKind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []NormalizedLocalEvent
	for rows.Next() {
		var event NormalizedLocalEvent
		if err := rows.Scan(&event.ID, &event.SourceKind, &event.SourcePath, &event.FileIdentity, &event.RecordStart, &event.RecordEnd, &event.EventType, &event.ObservedMS, &event.PayloadJSON, &event.CreatedMS); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func CommitNormalizedEvents(ctx context.Context, s Store, events []NormalizedLocalEvent, cursor LogCursor) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		for _, event := range events {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO normalized_local_events(
				id,source_kind,source_path,file_identity,record_start,record_end,event_type,observed_ms,payload_json,created_ms)
				VALUES(?,?,?,?,?,?,?,?,?,?)`, event.ID, event.SourceKind, event.SourcePath, event.FileIdentity,
				event.RecordStart, event.RecordEnd, event.EventType, event.ObservedMS, event.PayloadJSON, event.CreatedMS); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO log_cursors(
			source_kind,path,file_identity,generation,committed_offset,encoding,size,modified_ms,updated_ms)
			VALUES(?,?,?,?,?,?,?,?,?)
			ON CONFLICT(source_kind,path) DO UPDATE SET
				file_identity=excluded.file_identity,generation=excluded.generation,
				committed_offset=excluded.committed_offset,encoding=excluded.encoding,size=excluded.size,
				modified_ms=excluded.modified_ms,updated_ms=excluded.updated_ms`,
			cursor.SourceKind, cursor.Path, cursor.FileIdentity, cursor.Generation, cursor.CommittedOffset,
			cursor.Encoding, cursor.Size, cursor.ModifiedMS, cursor.UpdatedMS)
		return err
	})
}
