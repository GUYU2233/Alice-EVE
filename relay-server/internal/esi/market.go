package esi

import (
	"context"
	"fmt"
	"strconv"
)

// MarketOrder is the stable subset of an ESI regional order used by the app.
type MarketOrder struct {
	OrderID      int64   `json:"order_id"`
	TypeID       int64   `json:"type_id"`
	RegionID     int64   `json:"region_id"`
	SystemID     int64   `json:"system_id"`
	Price        float64 `json:"price"`
	VolumeRemain int64   `json:"volume_remain"`
	IsBuyOrder   bool    `json:"is_buy_order"`
	Duration     int     `json:"duration"`
	Issued       string  `json:"issued"`
}

// MarketOrders fetches public regional market orders; it never accepts account credentials.
func (s QueryService) MarketOrders(ctx context.Context, regionID, typeID int64) ([]MarketOrder, bool, error) {
	if regionID <= 0 || typeID <= 0 {
		return nil, false, fmt.Errorf("region and type IDs must be positive")
	}
	var result []MarketOrder
	cached, err := s.query(ctx, "markets/"+strconv.FormatInt(regionID, 10)+"/orders/", &result)
	if err != nil {
		return nil, cached, err
	}
	filtered := make([]MarketOrder, 0, len(result))
	for _, o := range result {
		if o.TypeID == typeID {
			filtered = append(filtered, o)
		}
	}
	return filtered, cached, nil
}
