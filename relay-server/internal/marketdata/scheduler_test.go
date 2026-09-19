package marketdata

import (
	"testing"
	"time"
)

func TestTierIntervalsAndBackoff(t *testing.T) {
	if !(tierInterval("A") < tierInterval("B") && tierInterval("B") < tierInterval("C") && tierInterval("C") < tierInterval("D")) {
		t.Fatal("tier intervals must increase A through D")
	}
	want := []time.Duration{time.Minute, 3 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour}
	for i, duration := range want {
		if got := failureBackoff(i + 1); got != duration {
			t.Fatalf("failure %d: got %v want %v", i+1, got, duration)
		}
	}
	if got := failureBackoff(99); got != 2*time.Hour {
		t.Fatalf("backoff cap = %v", got)
	}
}

func TestDeterministicJitterAndNextRun(t *testing.T) {
	at := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	first := deterministicJitter(10000002, at)
	second := deterministicJitter(10000002, at)
	if first != second {
		t.Fatal("jitter is not deterministic")
	}
	if first < -0.20 || first > 0.20 {
		t.Fatalf("jitter outside bounds: %f", first)
	}
	next := nextRun(10000002, at, time.Hour, 0)
	if next.Before(at.Add(48*time.Minute)) || next.After(at.Add(72*time.Minute)) {
		t.Fatalf("next run outside jitter bounds: %v", next)
	}
	if got := nextRun(10000002, at, time.Hour, 2); !got.Equal(at.Add(3 * time.Minute)) {
		t.Fatalf("failure next run = %v", got)
	}
}
