package app

import "time"

// TradeRegionStatus mirrors the scheduler-backed market region response. Pointer
// fields remain nil when talking to an older Relay that only returns state/error.
type TradeRegionStatus struct {
	RegionID  int64                `json:"regionId"`
	State     string               `json:"state,omitempty"`
	StartedAt time.Time            `json:"startedAt,omitempty"`
	EndedAt   *time.Time           `json:"endedAt,omitempty"`
	Snapshot  *TradeRegionSnapshot `json:"snapshot,omitempty"`
	Error     string               `json:"error,omitempty"`
	Schedule  *TradeRegionSchedule `json:"schedule,omitempty"`
	Job       *TradeRegionJob      `json:"job,omitempty"`
}

type TradeRegionSchedule struct {
	RegionID            int64         `json:"RegionID"`
	Enabled             bool          `json:"Enabled"`
	Tier                string        `json:"Tier"`
	Priority            int           `json:"Priority"`
	RefreshInterval     time.Duration `json:"RefreshInterval"`
	MinRefreshInterval  time.Duration `json:"MinRefreshInterval"`
	MaxStaleness        time.Duration `json:"MaxStaleness"`
	LastRequestedAt     time.Time     `json:"LastRequestedAt"`
	NextRunAt           time.Time     `json:"NextRunAt"`
	LastSuccessAt       time.Time     `json:"LastSuccessAt"`
	ConsecutiveFailures int           `json:"ConsecutiveFailures"`
	BackoffUntil        time.Time     `json:"BackoffUntil"`
	AccessScore         float64       `json:"AccessScore"`
	ChangeScore         float64       `json:"ChangeScore"`
}

type TradeRegionJob struct {
	ID                     string    `json:"ID"`
	RegionID               int64     `json:"RegionID"`
	State                  string    `json:"State"`
	Priority               int       `json:"Priority"`
	Trigger                string    `json:"Trigger"`
	RequestedAt            time.Time `json:"RequestedAt"`
	ClaimedAt              time.Time `json:"ClaimedAt"`
	ClaimExpiresAt         time.Time `json:"ClaimExpiresAt"`
	StartedAt              time.Time `json:"StartedAt"`
	CompletedAt            time.Time `json:"CompletedAt"`
	Attempt                int       `json:"Attempt"`
	FailureCode            string    `json:"FailureCode,omitempty"`
	FailureDetail          string    `json:"FailureDetail,omitempty"`
	RequestedByAccountID   string    `json:"-"`
	ClaimToken             string    `json:"-"`
}

type TradeRegionSnapshot struct {
	RegionID int64             `json:"RegionID"`
	Current  TradeRegionBatch  `json:"Current"`
	Previous *TradeRegionBatch `json:"Previous,omitempty"`
}

type TradeRegionBatch struct {
	ID             string    `json:"ID"`
	RegionID       int64     `json:"RegionID"`
	State          string    `json:"State"`
	ExpectedPages  int       `json:"ExpectedPages"`
	CollectedPages int       `json:"CollectedPages"`
	OrderCount     int64     `json:"OrderCount"`
	StartedAt      time.Time `json:"StartedAt"`
	CompletedAt    time.Time `json:"CompletedAt"`
	ExpiresAt      time.Time `json:"ExpiresAt"`
	ETag           string    `json:"ETag"`
	Failure        string    `json:"Failure,omitempty"`
}
