package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func newLocalIngestionTestStore(t *testing.T) (*SQLiteStore, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	store := NewSQLiteStore(db)
	ctx := context.Background()
	if err = EnsureReliableSchema(ctx, store); err != nil {
		t.Fatal(err)
	}
	return store, ctx
}

func TestLocalIngestionUpsertsAndLoadsCursor(t *testing.T) {
	store, ctx := newLocalIngestionTestStore(t)
	if err := UpsertLogSource(ctx, store, LogSource{SourceKind: "chat", RootPath: "old", UpdatedMS: 1}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLogSource(ctx, store, LogSource{SourceKind: "chat", RootPath: "new", UpdatedMS: 2}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLogFile(ctx, store, LogFile{SourceKind: "chat", Path: "a.log", FileIdentity: "f1", Generation: 1, Size: 10, ModifiedMS: 11, UpdatedMS: 12}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertLogFile(ctx, store, LogFile{SourceKind: "chat", Path: "a.log", FileIdentity: "f2", Generation: 2, Size: 20, ModifiedMS: 21, UpdatedMS: 22}); err != nil {
		t.Fatal(err)
	}
	cursor, found, err := LoadLogCursor(ctx, store, "chat", "missing.log")
	if err != nil || found {
		t.Fatalf("missing cursor: cursor=%+v found=%v err=%v", cursor, found, err)
	}
	want := LogCursor{SourceKind: "chat", Path: "a.log", FileIdentity: "f2", Generation: 2, CommittedOffset: 20, Encoding: "utf-8", Size: 20, ModifiedMS: 21, UpdatedMS: 23}
	if err = CommitNormalizedEvents(ctx, store, nil, want); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadLogCursor(ctx, store, "chat", "a.log")
	if err != nil || !found || got != want {
		t.Fatalf("cursor: got=%+v found=%v err=%v want=%+v", got, found, err, want)
	}
}

func TestCommitNormalizedEventsDeduplicatesAndIsAtomic(t *testing.T) {
	store, ctx := newLocalIngestionTestStore(t)
	event := NormalizedLocalEvent{ID: "stable-1", SourceKind: "chat", SourcePath: "a.log", FileIdentity: "f1", RecordStart: 0, RecordEnd: 8, EventType: "message", ObservedMS: 100, PayloadJSON: `{"text":"hi"}`, CreatedMS: 101}
	cursor := LogCursor{SourceKind: "chat", Path: "a.log", FileIdentity: "f1", Generation: 1, CommittedOffset: 8, Encoding: "utf-8", Size: 8, ModifiedMS: 99, UpdatedMS: 102}
	if err := CommitNormalizedEvents(ctx, store, []NormalizedLocalEvent{event, event}, cursor); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Query(ctx, "SELECT COUNT(*) FROM normalized_local_events WHERE id=?", event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("missing count row")
	}
	var count int
	if err = rows.Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err = store.Exec(ctx, `CREATE TRIGGER reject_test_event BEFORE INSERT ON normalized_local_events
		WHEN NEW.id='stable-2' BEGIN SELECT RAISE(ABORT, 'test rejection'); END`); err != nil {
		t.Fatal(err)
	}
	bad := event
	bad.ID = "stable-2"
	advanced := cursor
	advanced.CommittedOffset = 16
	if err = CommitNormalizedEvents(ctx, store, []NormalizedLocalEvent{bad}, advanced); err == nil {
		t.Fatal("expected constraint failure")
	}
	got, found, err := LoadLogCursor(ctx, store, cursor.SourceKind, cursor.Path)
	if err != nil || !found || got.CommittedOffset != cursor.CommittedOffset {
		t.Fatalf("transaction advanced cursor after failure: got=%+v found=%v err=%v", got, found, err)
	}
	rows2, err := store.Query(ctx, "SELECT id FROM normalized_local_events WHERE id=?", bad.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows2.Close()
	if rows2.Next() {
		t.Fatal(errors.New("failed transaction retained event"))
	}
}
