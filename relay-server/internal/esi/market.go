package esi

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type Status struct {
	Players       int       `json:"players"`
	ServerVersion string    `json:"server_version"`
	StartTime     time.Time `json:"start_time"`
	VIP           bool      `json:"vip"`
}

type SolarSystem struct {
	ConstellationID int64  `json:"constellation_id"`
	Name            string `json:"name"`
	Planets         []struct {
		PlanetID int64 `json:"planet_id"`
	} `json:"planets"`
	Position       Position `json:"position"`
	SecurityClass  string   `json:"security_class"`
	SecurityStatus float64  `json:"security_status"`
	StarID         int64    `json:"star_id"`
	Stargates      []int64  `json:"stargates"`
	Stations       []int64  `json:"stations"`
	SystemID       int64    `json:"system_id"`
}
type Position struct{ X, Y, Z float64 }
type Corporation struct {
	AllianceID    int64   `json:"alliance_id"`
	CEOID         int64   `json:"ceo_id"`
	CreatorID     int64   `json:"creator_id"`
	Description   string  `json:"description"`
	HomeStationID int64   `json:"home_station_id"`
	MemberCount   int64   `json:"member_count"`
	Name          string  `json:"name"`
	Shares        int64   `json:"shares"`
	TaxRate       float64 `json:"tax_rate"`
	Ticker        string  `json:"ticker"`
	URL           string  `json:"url"`
	WarEligible   bool    `json:"war_eligible"`
}
type Alliance struct {
	CreatorCorporationID  int64     `json:"creator_corporation_id"`
	CreatorID             int64     `json:"creator_id"`
	DateFounded           time.Time `json:"date_founded"`
	ExecutorCorporationID int64     `json:"executor_corporation_id"`
	FactionID             int64     `json:"faction_id"`
	Name                  string    `json:"name"`
	Ticker                string    `json:"ticker"`
}
type Station struct {
	MaxDockableShipVolume    float64  `json:"max_dockable_ship_volume"`
	Name                     string   `json:"name"`
	OfficeRentalCost         float64  `json:"office_rental_cost"`
	Owner                    int64    `json:"owner"`
	Position                 Position `json:"position"`
	RaceID                   int64    `json:"race_id"`
	ReprocessingEfficiency   float64  `json:"reprocessing_efficiency"`
	ReprocessingStationsTake float64  `json:"reprocessing_stations_take"`
	Services                 []string `json:"services"`
	StationID                int64    `json:"station_id"`
	SystemID                 int64    `json:"system_id"`
	TypeID                   int64    `json:"type_id"`
}
type ItemType struct {
	Capacity        float64          `json:"capacity"`
	Description     string           `json:"description"`
	DogmaAttributes []DogmaAttribute `json:"dogma_attributes"`
	DogmaEffects    []DogmaEffect    `json:"dogma_effects"`
	GraphicID       int64            `json:"graphic_id"`
	GroupID         int64            `json:"group_id"`
	IconID          int64            `json:"icon_id"`
	MarketGroupID   int64            `json:"market_group_id"`
	Mass            float64          `json:"mass"`
	Name            string           `json:"name"`
	PackagedVolume  float64          `json:"packaged_volume"`
	PortionSize     int              `json:"portion_size"`
	Published       bool             `json:"published"`
	Radius          float64          `json:"radius"`
	TypeID          int64            `json:"type_id"`
	Volume          float64          `json:"volume"`
}
type DogmaAttribute struct {
	AttributeID int64   `json:"attribute_id"`
	Value       float64 `json:"value"`
}
type DogmaEffect struct {
	EffectID  int64 `json:"effect_id"`
	IsDefault bool  `json:"is_default"`
}
type MarketPrice struct {
	AdjustedPrice float64 `json:"adjusted_price"`
	AveragePrice  float64 `json:"average_price"`
	TypeID        int64   `json:"type_id"`
}
type MarketOrder struct {
	Duration     int       `json:"duration"`
	IsBuyOrder   bool      `json:"is_buy_order"`
	Issued       time.Time `json:"issued"`
	LocationID   int64     `json:"location_id"`
	MinVolume    int64     `json:"min_volume"`
	OrderID      int64     `json:"order_id"`
	Price        float64   `json:"price"`
	Range        string    `json:"range"`
	SystemID     int64     `json:"system_id"`
	TypeID       int64     `json:"type_id"`
	VolumeRemain int64     `json:"volume_remain"`
	VolumeTotal  int64     `json:"volume_total"`
}
type MarketHistory struct {
	Average    float64 `json:"average"`
	Highest    float64 `json:"highest"`
	Lowest     float64 `json:"lowest"`
	Date       string  `json:"date"`
	OrderCount int64   `json:"order_count"`
	Volume     int64   `json:"volume"`
}

func (g *Gateway) Status(ctx context.Context) (Status, Response, error) {
	var v Status
	m, e := g.get(ctx, "status/", nil, "", &v)
	return v, m, e
}
func (g *Gateway) UniverseSystem(ctx context.Context, id int64) (SolarSystem, Response, error) {
	var v SolarSystem
	if e := positive("system ID", id); e != nil {
		return v, Response{}, e
	}
	m, e := g.get(ctx, idPath("universe/systems/", id, "/"), url.Values{"language": {"en"}}, "", &v)
	return v, m, e
}
func (g *Gateway) Corporation(ctx context.Context, id int64) (Corporation, Response, error) {
	var v Corporation
	if e := positive("corporation ID", id); e != nil {
		return v, Response{}, e
	}
	m, e := g.get(ctx, idPath("corporations/", id, "/"), nil, "", &v)
	return v, m, e
}
func (g *Gateway) Alliance(ctx context.Context, id int64) (Alliance, Response, error) {
	var v Alliance
	if e := positive("alliance ID", id); e != nil {
		return v, Response{}, e
	}
	m, e := g.get(ctx, idPath("alliances/", id, "/"), nil, "", &v)
	return v, m, e
}
func (g *Gateway) UniverseStation(ctx context.Context, id int64) (Station, Response, error) {
	var v Station
	if e := positive("station ID", id); e != nil {
		return v, Response{}, e
	}
	m, e := g.get(ctx, idPath("universe/stations/", id, "/"), nil, "", &v)
	return v, m, e
}
func (g *Gateway) UniverseType(ctx context.Context, id int64) (ItemType, Response, error) {
	var v ItemType
	if e := positive("type ID", id); e != nil {
		return v, Response{}, e
	}
	m, e := g.get(ctx, idPath("universe/types/", id, "/"), url.Values{"language": {"en"}}, "", &v)
	return v, m, e
}
func (g *Gateway) MarketPrices(ctx context.Context) ([]MarketPrice, Response, error) {
	var v []MarketPrice
	m, e := g.get(ctx, "markets/prices/", nil, "", &v)
	return v, m, e
}
func (g *Gateway) RegionOrders(ctx context.Context, regionID int64, orderType string, typeID int64) ([]MarketOrder, Response, error) {
	if e := positive("region ID", regionID); e != nil {
		return nil, Response{}, e
	}
	q := url.Values{}
	if orderType == "" {
		orderType = "all"
	}
	q.Set("order_type", orderType)
	if typeID > 0 {
		q.Set("type_id", strconv.FormatInt(typeID, 10))
	}
	return getAllPages[MarketOrder](ctx, g, idPath("markets/", regionID, "/orders/"), q, "")
}

// RegionOrdersPage fetches exactly one page. Full-region collectors use this
// method to keep response memory bounded and persist each page before fetching
// the next one.
func (g *Gateway) RegionOrdersPage(ctx context.Context, regionID int64, page int) ([]MarketOrder, Response, error) {
	if e := positive("region ID", regionID); e != nil {
		return nil, Response{}, e
	}
	if page < 1 || page > 10000 {
		return nil, Response{}, fmt.Errorf("page out of range")
	}
	var orders []MarketOrder
	meta, err := g.get(ctx, idPath("markets/", regionID, "/orders/"), url.Values{"order_type": {"all"}, "page": {strconv.Itoa(page)}}, "", &orders)
	return orders, meta, err
}
func (g *Gateway) RegionHistory(ctx context.Context, regionID, typeID int64) ([]MarketHistory, Response, error) {
	if e := positive("region ID", regionID); e != nil {
		return nil, Response{}, e
	}
	if e := positive("type ID", typeID); e != nil {
		return nil, Response{}, e
	}
	var v []MarketHistory
	m, e := g.get(ctx, idPath("markets/", regionID, "/history/"), url.Values{"type_id": {strconv.FormatInt(typeID, 10)}}, "", &v)
	return v, m, e
}
