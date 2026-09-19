package marketdata

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxClaimBatch       = 100
	MinClaimLease       = time.Second
	MaxClaimLease       = time.Hour
	MaxJobFailureCode   = 64
	MaxJobFailureDetail = 2048
)

var (
	ErrInvalidJob = errors.New("invalid market collection job")
	ErrClaimLost  = errors.New("market collection job claim lost")
)

type JobState string
type JobTrigger string

const (
	JobQueued     JobState = "queued"
	JobClaimed    JobState = "claimed"
	JobCollecting JobState = "collecting"
	JobPublishing JobState = "publishing"
	JobComplete   JobState = "complete"
	JobFailed     JobState = "failed"
	JobCanceled   JobState = "canceled"

	TriggerScheduler           JobTrigger = "scheduler"
	TriggerUserPriorityRefresh JobTrigger = "user_priority_refresh"
	TriggerStartupRecovery     JobTrigger = "startup_recovery"
	TriggerManualAdmin         JobTrigger = "manual_admin"
	TriggerStaleSnapshot       JobTrigger = "stale_snapshot"
)

type CollectionJob struct {
	ID, ClaimToken, RequestedByAccountID string
	RegionID                             int64
	State                                JobState
	Priority                             int
	Trigger                              JobTrigger
	RequestedAt, ClaimedAt               time.Time
	ClaimExpiresAt, StartedAt            time.Time
	CompletedAt                          time.Time
	Attempt                              int
	FailureCode, FailureDetail           string
}

type JobRepository interface {
	Enqueue(context.Context, int64, int, JobTrigger, string, time.Time) (CollectionJob, error)
	Claim(context.Context, time.Time, time.Duration, int) ([]CollectionJob, error)
	RenewLease(context.Context, string, string, time.Time, time.Duration) error
	CompleteJob(context.Context, string, string, time.Time) error
	FailJob(context.Context, string, string, time.Time, string, string) error
	RecoverExpired(context.Context, time.Time, int) (int64, error)
}

func validTrigger(t JobTrigger) bool {
	switch t {
	case TriggerScheduler, TriggerUserPriorityRefresh, TriggerStartupRecovery, TriggerManualAdmin, TriggerStaleSnapshot:
		return true
	default:
		return false
	}
}

func validateEnqueue(regionID int64, priority int, trigger JobTrigger, accountID string, at time.Time) error {
	if !validRegion(regionID) || priority < -1000000 || priority > 1000000 || !validTrigger(trigger) || at.IsZero() {
		return ErrInvalidJob
	}
	if strings.TrimSpace(accountID) != accountID {
		return ErrInvalidJob
	}
	return nil
}

func validLease(lease time.Duration) bool { return lease >= MinClaimLease && lease <= MaxClaimLease }

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
