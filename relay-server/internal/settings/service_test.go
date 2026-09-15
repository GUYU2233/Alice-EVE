package settings

import (
	"context"
	"errors"
	"testing"
)

func TestNewPolicyIgnoresBlankKeys(t *testing.T) {
	p := NewPolicy(" theme ", "", "   ", "notifications")
	if len(p.AllowedKeys) != 2 {
		t.Fatalf("expected two non-blank keys, got %#v", p.AllowedKeys)
	}
	if _, ok := p.AllowedKeys["theme"]; !ok {
		t.Fatal("expected trimmed theme key")
	}
	if _, ok := p.AllowedKeys[""]; ok {
		t.Fatal("blank key must not be allowed")
	}
}

func TestDefaultPolicyHasLimitsAndNoBlankKey(t *testing.T) {
	p := DefaultPolicy()
	if p.MaxBytes <= 0 || p.MaxDepth <= 0 || p.MaxKeys <= 0 || p.MaxArrayLength <= 0 {
		t.Fatalf("default policy must set all limits: %#v", p)
	}
	if _, ok := p.AllowedKeys[""]; ok {
		t.Fatal("default policy must not allow blank key")
	}
}

func TestUpdateUsesVersionAndETagAndDefensiveCopies(t *testing.T) {
	r := NewMemoryRepository()
	s := NewService(r, NewPolicy("theme", "notifications"))
	key := Key{AccountID: "acct", Scope: ScopeAccount}
	first, err := s.Get(context.Background(), key)
	if err != nil || first.Version != 1 {
		t.Fatalf("initial document: %#v %v", first, err)
	}
	updated, err := s.Update(context.Background(), UpdateRequest{Key: key, Values: map[string]any{"theme": "dark"}, BaseVersion: 1, IfMatch: first.ETag, UpdatedBy: "device"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.ETag == first.ETag {
		t.Fatalf("unexpected update: %#v", updated)
	}
	_, err = s.Update(context.Background(), UpdateRequest{Key: key, Values: map[string]any{"theme": "light"}, BaseVersion: 1, IfMatch: first.ETag})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Current.Version != 2 {
		t.Fatalf("expected conflict, got %v", err)
	}
	updated.Values["theme"] = "mutated"
	got, _ := s.Get(context.Background(), key)
	if got.Values["theme"] != "dark" {
		t.Fatalf("repository leaked mutable document: %#v", got.Values)
	}
}

func TestUpdateRequiresBothVersionAndIfMatch(t *testing.T) {
	r := NewMemoryRepository()
	s := NewService(r, NewPolicy("theme"))
	key := Key{AccountID: "acct", Scope: ScopeAccount}
	d, err := s.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	cases := []UpdateRequest{
		{Key: key, Values: map[string]any{"theme": "dark"}, BaseVersion: d.Version, IfMatch: ""},
		{Key: key, Values: map[string]any{"theme": "dark"}, BaseVersion: d.Version + 1, IfMatch: d.ETag},
	}
	for i, req := range cases {
		if _, err := s.Update(context.Background(), req); !errors.Is(err, ErrSettingsConflict) {
			t.Fatalf("case %d: expected conflict, got %v", i, err)
		}
	}
}

func TestSettingsWhitelistAndSecretRejection(t *testing.T) {
	s := NewService(NewMemoryRepository(), NewPolicy("theme"))
	key := Key{AccountID: "acct", Scope: ScopeAccount}
	d, _ := s.Get(context.Background(), key)
	if _, err := s.Update(context.Background(), UpdateRequest{Key: key, Values: map[string]any{"unknown": true}, BaseVersion: d.Version, IfMatch: d.ETag}); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("expected whitelist error, got %v", err)
	}
	if _, err := s.Update(context.Background(), UpdateRequest{Key: key, Values: map[string]any{"theme": "https://secret.invalid"}, BaseVersion: d.Version, IfMatch: d.ETag}); !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("expected URL rejection, got %v", err)
	}
}

func TestResetIncrementsVersion(t *testing.T) {
	r := NewMemoryRepository()
	s := NewService(r, NewPolicy("theme"))
	key := Key{AccountID: "acct", Scope: ScopeAccount}
	d, _ := s.Get(context.Background(), key)
	d, err := s.Update(context.Background(), UpdateRequest{Key: key, Values: map[string]any{"theme": "dark"}, BaseVersion: d.Version, IfMatch: d.ETag})
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.Reset(context.Background(), key, d.Version, d.ETag, "device")
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 3 || len(d.Values) != 0 {
		t.Fatalf("unexpected reset %#v", d)
	}
}
