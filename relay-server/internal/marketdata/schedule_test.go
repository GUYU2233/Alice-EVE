package marketdata

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestValidateEnqueueBoundaries(t *testing.T) {
	now := time.Now()
	valid := []JobTrigger{TriggerScheduler, TriggerUserPriorityRefresh, TriggerStartupRecovery, TriggerManualAdmin, TriggerStaleSnapshot}
	for _, trigger := range valid {
		if err := validateEnqueue(10000002, 0, trigger, "", now); err != nil {
			t.Fatalf("valid trigger %q: %v", trigger, err)
		}
	}
	cases := []struct {
		region   int64
		priority int
		trigger  JobTrigger
		account  string
		at       time.Time
	}{
		{0, 0, TriggerScheduler, "", now},
		{MaxUniverseID + 1, 0, TriggerScheduler, "", now},
		{1, -1000001, TriggerScheduler, "", now},
		{1, 1000001, TriggerScheduler, "", now},
		{1, 0, "unknown", "", now},
		{1, 0, TriggerScheduler, " padded ", now},
		{1, 0, TriggerScheduler, "", time.Time{}},
	}
	for i, tc := range cases {
		if err := validateEnqueue(tc.region, tc.priority, tc.trigger, tc.account, tc.at); !errors.Is(err, ErrInvalidJob) {
			t.Errorf("case %d got %v, want ErrInvalidJob", i, err)
		}
	}
}

func TestClaimResourceBounds(t *testing.T) {
	if !validLease(MinClaimLease) || !validLease(MaxClaimLease) {
		t.Fatal("lease boundary must be accepted")
	}
	if validLease(MinClaimLease-time.Nanosecond) || validLease(MaxClaimLease+time.Nanosecond) {
		t.Fatal("lease outside hard bounds must be rejected")
	}
}

func TestTruncateUTF8(t *testing.T) {
	detail := strings.Repeat("界", MaxJobFailureDetail)
	got := truncateUTF8(detail, MaxJobFailureDetail)
	if len(got) > MaxJobFailureDetail || !utf8.ValidString(got) {
		t.Fatalf("invalid truncation: bytes=%d valid=%v", len(got), utf8.ValidString(got))
	}
	if got := truncateUTF8("short", MaxJobFailureDetail); got != "short" {
		t.Fatalf("unexpected short value %q", got)
	}
}
