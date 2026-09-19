package app

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// TradeBudget separates the user's spend ceiling from the observed wallet.
// A nil MaximumISK means "use wallet balance"; ReserveISK is never spendable.
type TradeBudget struct {
	MaximumISK *float64 `json:"maximumISK,omitempty"`
	ReserveISK float64  `json:"reserveISK"`
}

func (b TradeBudget) Available(wallet float64) (float64, error) {
	if wallet < 0 || b.ReserveISK < 0 || (b.MaximumISK != nil && *b.MaximumISK < 0) {
		return 0, fmt.Errorf("wallet, reserve, and maximum budget must be non-negative")
	}
	available := math.Max(0, wallet-b.ReserveISK)
	if b.MaximumISK != nil && *b.MaximumISK < available {
		available = *b.MaximumISK
	}
	return available, nil
}

type CargoCapacity struct {
	M3        float64 `json:"m3"`
	Estimated bool    `json:"estimated"`
	Source    string  `json:"source"`
}

type CharacterTradeContext struct {
	CharacterID        int64         `json:"characterID"`
	SolarSystemID      int64         `json:"solarSystemID"`
	StationID          int64         `json:"stationID,omitempty"`
	StructureID        int64         `json:"structureID,omitempty"`
	ShipItemID         int64         `json:"shipItemID"`
	ShipTypeID         int64         `json:"shipTypeID"`
	ShipName           string        `json:"shipName"`
	Cargo              CargoCapacity `json:"cargo"`
	WalletISK          float64       `json:"walletISK"`
	AvailableBudgetISK float64       `json:"availableBudgetISK"`
	ObservedAt         time.Time     `json:"observedAt"`
	Stale              bool          `json:"stale"`
}

type shipCapacityResolver interface {
	ShipCargoCapacity(typeID int64) (m3 float64, source string, exact bool, err error)
}

// BuildCharacterTradeContext normalizes Relay location/ship/wallet snapshots.
// The resolver deliberately owns cargo semantics: use an exact fitted-ship
// source when available, otherwise base type capacity must be labelled estimated.
func BuildCharacterTradeContext(snapshots []CharacterSnapshot, budget TradeBudget, capacity shipCapacityResolver) (CharacterTradeContext, error) {
	var out CharacterTradeContext
	found := map[string]bool{}
	var observed time.Time
	for _, s := range snapshots {
		if out.CharacterID == 0 {
			out.CharacterID = s.CharacterID
		} else if s.CharacterID != out.CharacterID {
			return out, fmt.Errorf("snapshots contain multiple characters")
		}
		if s.FetchedAt.After(observed) {
			observed = s.FetchedAt
		}
		out.Stale = out.Stale || s.Stale
		switch strings.ToLower(s.Domain) {
		case "location":
			var v struct {
				SolarSystemID int64 `json:"solar_system_id"`
				StationID     int64 `json:"station_id"`
				StructureID   int64 `json:"structure_id"`
			}
			if err := json.Unmarshal(s.Payload, &v); err != nil {
				return out, fmt.Errorf("location snapshot: %w", err)
			}
			out.SolarSystemID, out.StationID, out.StructureID = v.SolarSystemID, v.StationID, v.StructureID
			found["location"] = true
		case "ship":
			var v struct {
				ShipItemID int64  `json:"ship_item_id"`
				ShipTypeID int64  `json:"ship_type_id"`
				ShipName   string `json:"ship_name"`
			}
			if err := json.Unmarshal(s.Payload, &v); err != nil {
				return out, fmt.Errorf("ship snapshot: %w", err)
			}
			out.ShipItemID, out.ShipTypeID, out.ShipName = v.ShipItemID, v.ShipTypeID, v.ShipName
			found["ship"] = true
		case "wallet":
			if err := json.Unmarshal(s.Payload, &out.WalletISK); err != nil {
				return out, fmt.Errorf("wallet snapshot: %w", err)
			}
			found["wallet"] = true
		}
	}
	for _, d := range []string{"location", "ship", "wallet"} {
		if !found[d] {
			return out, fmt.Errorf("required %s snapshot missing", d)
		}
	}
	if out.SolarSystemID <= 0 || out.ShipTypeID <= 0 {
		return out, fmt.Errorf("location or ship snapshot is incomplete")
	}
	if capacity == nil {
		return out, fmt.Errorf("ship cargo capacity resolver is required")
	}
	m3, source, exact, err := capacity.ShipCargoCapacity(out.ShipTypeID)
	if err != nil {
		return out, err
	}
	if m3 < 0 || strings.TrimSpace(source) == "" {
		return out, fmt.Errorf("invalid cargo capacity result")
	}
	out.Cargo = CargoCapacity{M3: m3, Estimated: !exact, Source: source}
	out.AvailableBudgetISK, err = budget.Available(out.WalletISK)
	out.ObservedAt = observed
	return out, err
}
