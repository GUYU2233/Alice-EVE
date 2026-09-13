package storage

import (
	"context"
	"database/sql"
	"errors"
	"eve-assistant/desktop-app/internal/protocol"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

func TestPairingRoundTrip(t *testing.T) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := NewSQLiteStore(db)
	ctx := context.Background()
	if e = EnsureReliableSchema(ctx, s); e != nil {
		t.Fatal(e)
	}
	p := PairingState{RelayURL: "https://relay", DeviceID: "d", PairedAt: time.Unix(10, 0), Version: 1}
	if e = SavePairing(ctx, s, p); e != nil {
		t.Fatal(e)
	}
	q, e := LoadPairing(ctx, s)
	if e != nil || q.DeviceID != "d" {
		t.Fatalf("%v %+v", e, q)
	}
	if e = ClearPairing(ctx, s); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadPairing(ctx, s); !errors.Is(e, ErrPairingNotFound) {
		t.Fatalf("expected not found, got %v", e)
	}
	_, e = Enqueue(ctx, s, protocol.EventEnvelope{MessageID: "x"})
	if e != nil {
		t.Fatal(e)
	}
	if e = Ack(ctx, s, "x"); e != nil {
		t.Fatal(e)
	}
}

func TestSavePairingRejectsIncompleteState(t *testing.T) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := NewSQLiteStore(db)
	ctx := context.Background()
	if e = EnsureReliableSchema(ctx, s); e != nil {
		t.Fatal(e)
	}
	if e = SavePairing(ctx, s, PairingState{RelayURL: "https://relay"}); e == nil {
		t.Fatal("expected validation error")
	}
}
