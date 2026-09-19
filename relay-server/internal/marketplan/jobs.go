package marketplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var ErrInvalidInput = errors.New("invalid market plan job input")
var ErrNotFound = errors.New("market plan job not found")

type Constraints struct {
	Budget           float64 `json:"budget"`
	BudgetReserve    float64 `json:"budgetReserve"`
	CargoM3          float64 `json:"cargoM3"`
	MinSecurity      float64 `json:"minSecurity"`
	MaxJumps         int     `json:"maxJumps"`
	TargetLoadFactor float64 `json:"targetLoadFactor"`
}
type CreateRequest struct {
	CharacterID          int64       `json:"characterId"`
	Mode                 string      `json:"mode"`
	SourceRegionIDs      []int64     `json:"sourceRegionIds"`
	DestinationScope     string      `json:"destinationScope"`
	DestinationRegionIDs []int64     `json:"destinationRegionIds,omitempty"`
	Constraints          Constraints `json:"constraints"`
}
type Job struct {
	ID                   string          `json:"id"`
	AccountID            string          `json:"accountId"`
	Mode                 string          `json:"mode"`
	DestinationScope     string          `json:"destinationScope"`
	ConstraintHash       string          `json:"constraintHash"`
	SnapshotSignature    string          `json:"snapshotSignature"`
	State                string          `json:"state"`
	LastError            string          `json:"lastError"`
	CharacterID          int64           `json:"characterId"`
	SourceRegionIDs      []int64         `json:"sourceRegionIds"`
	DestinationRegionIDs []int64         `json:"destinationRegionIds"`
	Constraints          Constraints     `json:"constraints"`
	Progress             json.RawMessage `json:"progress"`
	Iteration            int64           `json:"iteration"`
	ResultRevision       int64           `json:"resultRevision"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	StartedAt            *time.Time      `json:"startedAt,omitempty"`
	CompletedAt          *time.Time      `json:"completedAt,omitempty"`
}
type Result struct {
	Revision  int64           `json:"revision"`
	Rank      int             `json:"rank"`
	StableKey string          `json:"stableKey"`
	Score     float64         `json:"score"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}
type Repository interface {
	Create(context.Context, string, CreateRequest, string) (Job, error)
	Get(context.Context, string, string) (Job, error)
	ResultsAfter(context.Context, string, string, int64) ([]Result, error)
	Cancel(context.Context, string, string) (Job, error)
}

func Normalize(q *CreateRequest) (string, error) {
	q.Mode = strings.ToLower(strings.TrimSpace(q.Mode))
	q.DestinationScope = strings.TrimSpace(q.DestinationScope)
	if q.DestinationScope == "" {
		q.DestinationScope = "all_collected_regions"
	}
	if q.SourceRegionIDs == nil {
		q.SourceRegionIDs = []int64{}
	}
	if q.DestinationRegionIDs == nil {
		q.DestinationRegionIDs = []int64{}
	}
	if q.Constraints.TargetLoadFactor == 0 {
		q.Constraints.TargetLoadFactor = .9
	}
	sort.Slice(q.SourceRegionIDs, func(i, j int) bool { return q.SourceRegionIDs[i] < q.SourceRegionIDs[j] })
	sort.Slice(q.DestinationRegionIDs, func(i, j int) bool { return q.DestinationRegionIDs[i] < q.DestinationRegionIDs[j] })
	if q.CharacterID < 0 || (q.Mode != "single" && q.Mode != "basket" && q.Mode != "chain") || len(q.SourceRegionIDs) < 1 || len(q.SourceRegionIDs) > 64 || (q.DestinationScope != "all_collected_regions" && q.DestinationScope != "selected_regions") || (q.DestinationScope == "selected_regions" && len(q.DestinationRegionIDs) == 0) || q.Constraints.Budget <= 0 || q.Constraints.BudgetReserve < 0 || q.Constraints.BudgetReserve >= q.Constraints.Budget || q.Constraints.CargoM3 <= 0 || q.Constraints.MinSecurity < -1 || q.Constraints.MinSecurity > 1 || q.Constraints.MaxJumps < 0 || q.Constraints.MaxJumps > 256 || q.Constraints.TargetLoadFactor <= 0 || q.Constraints.TargetLoadFactor > 1 || math.IsNaN(q.Constraints.Budget) || math.IsInf(q.Constraints.Budget, 0) {
		return "", ErrInvalidInput
	}
	for _, ids := range [][]int64{q.SourceRegionIDs, q.DestinationRegionIDs} {
		seen := map[int64]bool{}
		for _, id := range ids {
			if id <= 0 || id > 2147483647 || seen[id] {
				return "", ErrInvalidInput
			}
			seen[id] = true
		}
	}
	b, err := json.Marshal(q)
	if err != nil {
		return "", ErrInvalidInput
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
