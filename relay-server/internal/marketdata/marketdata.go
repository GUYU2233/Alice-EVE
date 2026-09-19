// Package marketdata owns durable, normalized full-region market collection.
// Unpublished batches are never visible through Snapshot; Publish atomically
// advances the region pointer while retaining the immediately previous snapshot.
package marketdata

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MaxUniverseID    int64 = 2147483647
	MaxOrdersPerPage       = 10000
	MaxPagesPerBatch       = 10000
)

var (
	ErrInvalidInput    = errors.New("invalid market data input")
	ErrNotFound        = errors.New("market snapshot not found")
	ErrIncompleteBatch = errors.New("market batch is incomplete")
	ErrBatchState      = errors.New("market batch is not collecting")
)

type Batch struct {
	ID             string
	RegionID       int64
	State          string
	ExpectedPages  int
	CollectedPages int
	OrderCount     int64
	StartedAt      time.Time
	CompletedAt    time.Time
	ExpiresAt      time.Time
	ETag           string
	Failure        string
}

type Order struct {
	OrderID, TypeID, LocationID, SystemID int64
	IsBuyOrder                            bool
	Price                                 float64
	VolumeTotal, VolumeRemain, MinVolume  int64
	Range                                 string
	Duration                              int
	Issued                                time.Time
}

type Snapshot struct {
	RegionID int64
	Current  Batch
	Previous *Batch
}

type Repository interface {
	BeginBatch(context.Context, int64, time.Time) (Batch, error)
	AppendPage(context.Context, string, int, int, []Order) error
	Publish(context.Context, string, time.Time, time.Time, string) (Snapshot, error)
	Fail(context.Context, string, string) error
	Snapshot(context.Context, int64) (Snapshot, error)
}

func validRegion(id int64) bool { return id > 0 && id <= MaxUniverseID }
func invalidOrderReason(o Order) string {
	switch {
	case o.OrderID <= 0:
		return "order_id"
	case o.TypeID <= 0 || o.TypeID > MaxUniverseID:
		return "type_id"
	case o.LocationID <= 0:
		return "location_id"
	case o.SystemID <= 0 || o.SystemID > MaxUniverseID:
		return "system_id"
	case math.IsNaN(o.Price) || math.IsInf(o.Price, 0) || o.Price < 0:
		return "price"
	case o.VolumeTotal < 0 || o.VolumeRemain < 0 || o.VolumeRemain > o.VolumeTotal:
		return "volume"
	case o.MinVolume <= 0:
		return "min_volume"
	case len(strings.TrimSpace(o.Range)) == 0 || len(o.Range) > 32:
		return "range"
	case o.Duration <= 0:
		return "duration"
	case o.Issued.IsZero():
		return "issued"
	default:
		return ""
	}
}
func validOrder(o Order) bool { return invalidOrderReason(o) == "" }

func validatePage(page, expected int, orders []Order) error {
	if page < 1 || expected < 1 || page > expected || expected > MaxPagesPerBatch || len(orders) > MaxOrdersPerPage {
		return ErrInvalidInput
	}
	seen := make(map[int64]struct{}, len(orders))
	for i, order := range orders {
		if reason := invalidOrderReason(order); reason != "" {
			return fmt.Errorf("%w: order index %d id %d invalid %s", ErrInvalidInput, i, order.OrderID, reason)
		}
		if _, exists := seen[order.OrderID]; exists {
			return fmt.Errorf("%w: duplicate order ID", ErrInvalidInput)
		}
		seen[order.OrderID] = struct{}{}
	}
	return nil
}
