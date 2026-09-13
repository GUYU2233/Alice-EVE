package app

import (
	"testing"
	"time"
)

func TestParseIntelDeterministic(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := ParseIntel("Hostile Rifter spotted in Jita", "clipboard", now)
	if len(got) != 3 {
		t.Fatalf("got %d findings, want 3: %#v", len(got), got)
	}
	if got[0].Value != "Jita" || got[1].Value != "Rifter" || got[2].Value != "hostile" {
		t.Fatalf("unexpected findings: %#v", got)
	}
	if !got[0].ExpiresAt.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("unexpected expiry: %v", got[0].ExpiresAt)
	}
	if got[0].Source != "clipboard" {
		t.Fatalf("unexpected source: %q", got[0].Source)
	}
}

func TestParseIntelDoesNotInvent(t *testing.T) {
	if got := ParseIntel("quiet channel", "clipboard", time.Time{}); len(got) != 0 {
		t.Fatalf("unexpected findings: %#v", got)
	}
}
