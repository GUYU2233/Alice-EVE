package esidata

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCharacterSnapshotsUpsertGetListAndIsolation(t *testing.T) {
	repo := NewMemory()
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	base := CharacterSnapshot{
		AccountID:   "account-a",
		CharacterID: 9001,
		Domain:      "skills",
		Payload:     []byte(`{"total_sp":100}`),
		FetchedAt:   now,
		ExpiresAt:   now.Add(time.Hour),
		Source:      "esi",
		ETag:        `"one"`,
	}
	if err := repo.UpsertCharacterSnapshot(ctx, base); err != nil {
		t.Fatal(err)
	}

	// Stored and returned payloads must not alias caller-owned memory.
	base.Payload[2] = 'X'
	got, err := repo.GetCharacterSnapshot(ctx, "account-a", 9001, "skills")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) != `{"total_sp":100}` {
		t.Fatalf("payload = %s", got.Payload)
	}
	got.Payload[2] = 'Y'
	got, err = repo.GetCharacterSnapshot(ctx, "account-a", 9001, "skills")
	if err != nil || string(got.Payload) != `{"total_sp":100}` {
		t.Fatalf("payload alias or get failure: %s, %v", got.Payload, err)
	}

	updated := got
	updated.Payload = []byte(`{"total_sp":200}`)
	updated.Stale = true
	updated.ETag = `"two"`
	if err := repo.UpsertCharacterSnapshot(ctx, updated); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertCharacterSnapshot(ctx, CharacterSnapshot{
		AccountID: "account-a", CharacterID: 9001, Domain: "assets",
		Payload: []byte(`[]`), FetchedAt: now, ExpiresAt: now.Add(time.Hour), Source: "esi",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertCharacterSnapshot(ctx, CharacterSnapshot{
		AccountID: "account-b", CharacterID: 9001, Domain: "wallet",
		Payload: []byte(`{"balance":1}`), FetchedAt: now, ExpiresAt: now.Add(time.Hour), Source: "esi",
	}); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListCharacterDomains(ctx, "account-a", 9001)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Domain != "assets" || list[1].Domain != "skills" {
		t.Fatalf("unexpected domain list: %+v", list)
	}
	if string(list[1].Payload) != `{"total_sp":200}` || !list[1].Stale {
		t.Fatalf("upsert did not replace snapshot: %+v", list[1])
	}
	if _, err := repo.GetCharacterSnapshot(ctx, "account-b", 9001, "skills"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account get error = %v", err)
	}
}

func TestMemoryPublicDataUpsertGet(t *testing.T) {
	repo := NewMemory()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	entry := PublicData{
		Kind: "types", CacheKey: "34", Payload: []byte(`{"name":"Tritanium"}`),
		FetchedAt: now, ExpiresAt: now.Add(24 * time.Hour), ETag: `"v1"`,
	}
	if err := repo.UpsertPublicData(ctx, entry); err != nil {
		t.Fatal(err)
	}
	entry.Payload = []byte(`{"name":"Changed"}`)
	got, err := repo.GetPublicData(ctx, "types", "34")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Payload) != `{"name":"Tritanium"}` {
		t.Fatalf("payload = %s", got.Payload)
	}
	got.Payload = []byte(`[]`)
	got.ETag = `"v2"`
	if err := repo.UpsertPublicData(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetPublicData(ctx, "types", "34")
	if err != nil || string(got.Payload) != `[]` || got.ETag != `"v2"` {
		t.Fatalf("upsert result = %+v, %v", got, err)
	}
}

func TestMemoryRejectsInvalidInputAndHonorsContext(t *testing.T) {
	repo := NewMemory()
	now := time.Now().UTC()
	invalid := []CharacterSnapshot{
		{},
		{AccountID: "a", CharacterID: 1, Domain: "d", Source: "esi", Payload: []byte(`null`), FetchedAt: now, ExpiresAt: now.Add(time.Hour)},
		{AccountID: "a", CharacterID: 1, Domain: "d", Source: "esi", Payload: []byte(`{`), FetchedAt: now, ExpiresAt: now.Add(time.Hour)},
		{AccountID: "a", CharacterID: 1, Domain: "d", Source: "esi", Payload: []byte(`{}`), FetchedAt: now, ExpiresAt: now.Add(-time.Hour)},
	}
	for _, snapshot := range invalid {
		if err := repo.UpsertCharacterSnapshot(context.Background(), snapshot); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid snapshot error = %v", err)
		}
	}
	if err := repo.UpsertPublicData(context.Background(), PublicData{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid public data error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repo.GetPublicData(ctx, "types", "34"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v", err)
	}
}
