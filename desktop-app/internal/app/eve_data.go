package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const evePublicPath = "/api/v1/eve/public/"

// CharacterSnapshot is normalized Relay-owned character data. Payload contains
// EVE data only; upstream access and refresh tokens are never returned.
type CharacterSnapshot struct {
	AccountID   string          `json:"AccountID"`
	CharacterID int64           `json:"CharacterID"`
	Domain      string          `json:"Domain"`
	Payload     json.RawMessage `json:"Payload"`
	FetchedAt   time.Time       `json:"FetchedAt"`
	ExpiresAt   time.Time       `json:"ExpiresAt"`
	Stale       bool            `json:"Stale"`
	Source      string          `json:"Source"`
	ETag        string          `json:"ETag"`
}

type EVEStatus struct {
	Players       int       `json:"players"`
	ServerVersion string    `json:"server_version"`
	StartTime     time.Time `json:"start_time"`
	VIP           bool      `json:"vip"`
}

type EVEPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type EVESolarSystem struct {
	ConstellationID int64  `json:"constellation_id"`
	Name            string `json:"name"`
	Planets         []struct {
		PlanetID int64 `json:"planet_id"`
	} `json:"planets"`
	Position       EVEPosition `json:"position"`
	SecurityClass  string      `json:"security_class"`
	SecurityStatus float64     `json:"security_status"`
	StarID         int64       `json:"star_id"`
	Stargates      []int64     `json:"stargates"`
	Stations       []int64     `json:"stations"`
	SystemID       int64       `json:"system_id"`
}

type EVECorporation struct {
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
type EVEAlliance struct {
	CreatorCorporationID  int64     `json:"creator_corporation_id"`
	CreatorID             int64     `json:"creator_id"`
	DateFounded           time.Time `json:"date_founded"`
	ExecutorCorporationID int64     `json:"executor_corporation_id"`
	FactionID             int64     `json:"faction_id"`
	Name                  string    `json:"name"`
	Ticker                string    `json:"ticker"`
}
type EVEStation struct {
	MaxDockableShipVolume    float64     `json:"max_dockable_ship_volume"`
	Name                     string      `json:"name"`
	OfficeRentalCost         float64     `json:"office_rental_cost"`
	Owner                    int64       `json:"owner"`
	Position                 EVEPosition `json:"position"`
	RaceID                   int64       `json:"race_id"`
	ReprocessingEfficiency   float64     `json:"reprocessing_efficiency"`
	ReprocessingStationsTake float64     `json:"reprocessing_stations_take"`
	Services                 []string    `json:"services"`
	StationID                int64       `json:"station_id"`
	SystemID                 int64       `json:"system_id"`
	TypeID                   int64       `json:"type_id"`
}

type EVEDogmaAttribute struct {
	AttributeID int64   `json:"attribute_id"`
	Value       float64 `json:"value"`
}
type EVEDogmaEffect struct {
	EffectID  int64 `json:"effect_id"`
	IsDefault bool  `json:"is_default"`
}

type EVEItemType struct {
	Capacity        float64             `json:"capacity"`
	Description     string              `json:"description"`
	DogmaAttributes []EVEDogmaAttribute `json:"dogma_attributes"`
	DogmaEffects    []EVEDogmaEffect    `json:"dogma_effects"`
	GraphicID       int64               `json:"graphic_id"`
	GroupID         int64               `json:"group_id"`
	IconID          int64               `json:"icon_id"`
	MarketGroupID   int64               `json:"market_group_id"`
	Mass            float64             `json:"mass"`
	Name            string              `json:"name"`
	PackagedVolume  float64             `json:"packaged_volume"`
	PortionSize     int                 `json:"portion_size"`
	Published       bool                `json:"published"`
	Radius          float64             `json:"radius"`
	TypeID          int64               `json:"type_id"`
	Volume          float64             `json:"volume"`
}

type EVESyncJob struct {
	ID        string    `json:"ID"`
	AccountID string    `json:"AccountID"`
	Kind      string    `json:"Kind"`
	Status    string    `json:"Status"`
	RunAfter  time.Time `json:"RunAfter"`
	Attempts  int       `json:"Attempts"`
}

type EVEUniverseName struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type EVETradeHub struct {
	Code      string `json:"Code"`
	Name      string `json:"Name"`
	NameZH    string `json:"NameZH"`
	RegionID  int64  `json:"RegionID"`
	StationID int64  `json:"StationID"`
	SystemID  int64  `json:"SystemID"`
}
type EVEHubQuote struct {
	Hub           EVETradeHub `json:"hub"`
	TypeID        int64       `json:"typeId"`
	BestBuy       float64     `json:"bestBuy"`
	BestSell      float64     `json:"bestSell"`
	Spread        float64     `json:"spread"`
	SpreadPercent float64     `json:"spreadPercent"`
	BuyVolume     int64       `json:"buyVolume"`
	SellVolume    int64       `json:"sellVolume"`
	BuyOrders     int         `json:"buyOrders"`
	SellOrders    int         `json:"sellOrders"`
	FetchedAt     time.Time   `json:"fetchedAt"`
	ExpiresAt     time.Time   `json:"expiresAt"`
}
type EVETradePlanRequest struct {
	TypeID             int64   `json:"TypeID"`
	Quantity           int64   `json:"Quantity"`
	Source             string  `json:"Source"`
	Destination        string  `json:"Destination"`
	SalesTaxRate       float64 `json:"SalesTaxRate"`
	BrokerRate         float64 `json:"BrokerRate"`
	TransportCostPerM3 float64 `json:"TransportCostPerM3"`
	CargoM3            float64 `json:"CargoM3"`
	ItemVolumeM3       float64 `json:"ItemVolumeM3"`
	RouteJumps         int     `json:"RouteJumps"`
	LowSecJumps        int     `json:"LowSecJumps"`
}

type EVEMarketPrice struct {
	AdjustedPrice float64 `json:"adjusted_price"`
	AveragePrice  float64 `json:"average_price"`
	TypeID        int64   `json:"type_id"`
}
type EVEMarketOrder struct {
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
type EVEMarketHistory struct {
	Average    float64 `json:"average"`
	Highest    float64 `json:"highest"`
	Lowest     float64 `json:"lowest"`
	Date       string  `json:"date"`
	OrderCount int64   `json:"order_count"`
	Volume     int64   `json:"volume"`
}

func positiveEVEID(name string, id int64) error {
	if id <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func (r *RelayClient) FetchEVESyncStatus(ctx context.Context) ([]EVESyncJob, error) {
	var jobs []EVESyncJob
	err := r.getJSON(ctx, "/api/v1/eve/sync-status", &jobs)
	return jobs, err
}

func (r *RelayClient) FetchAccountCharacterSnapshots(ctx context.Context) ([]CharacterSnapshot, error) {
	var snapshots []CharacterSnapshot
	err := r.getJSON(ctx, "/api/v1/eve/characters", &snapshots)
	return snapshots, err
}

func (r *RelayClient) FetchCharacterSnapshots(ctx context.Context, characterID int64) ([]CharacterSnapshot, error) {
	if err := positiveEVEID("character id", characterID); err != nil {
		return nil, err
	}
	var snapshots []CharacterSnapshot
	err := r.getJSON(ctx, "/api/v1/eve/characters/"+strconv.FormatInt(characterID, 10), &snapshots)
	return snapshots, err
}

func (r *RelayClient) FetchCharacterSnapshot(ctx context.Context, characterID int64, domain string) (CharacterSnapshot, error) {
	if err := positiveEVEID("character id", characterID); err != nil {
		return CharacterSnapshot{}, err
	}
	domain = strings.TrimSpace(domain)
	if domain == "" || strings.ContainsAny(domain, "/\\") {
		return CharacterSnapshot{}, fmt.Errorf("character domain is invalid")
	}
	var snapshot CharacterSnapshot
	err := r.getJSON(ctx, "/api/v1/eve/characters/"+strconv.FormatInt(characterID, 10)+"/"+url.PathEscape(domain), &snapshot)
	return snapshot, err
}

func (r *RelayClient) FetchEVEContractItems(ctx context.Context, characterID, contractID int64) ([]map[string]any, error) {
	return r.fetchEVEDetailList(ctx, fmt.Sprintf("/api/v1/eve/details/contracts/%d/%d/items", characterID, contractID))
}
func (r *RelayClient) FetchEVEContractBids(ctx context.Context, characterID, contractID int64) ([]map[string]any, error) {
	return r.fetchEVEDetailList(ctx, fmt.Sprintf("/api/v1/eve/details/contracts/%d/%d/bids", characterID, contractID))
}
func (r *RelayClient) FetchEVEMailBody(ctx context.Context, characterID, mailID int64) (map[string]any, error) {
	var v map[string]any
	err := r.getJSON(ctx, fmt.Sprintf("/api/v1/eve/details/mail/%d/%d", characterID, mailID), &v)
	return v, err
}
func (r *RelayClient) FetchEVEKillmailDetail(ctx context.Context, characterID, killmailID int64, hash string) (map[string]any, error) {
	var v map[string]any
	err := r.getJSON(ctx, fmt.Sprintf("/api/v1/eve/details/killmails/%d/%d/%s", characterID, killmailID, url.PathEscape(hash)), &v)
	return v, err
}
func (r *RelayClient) fetchEVEDetailList(ctx context.Context, path string) ([]map[string]any, error) {
	var v []map[string]any
	err := r.getJSON(ctx, path, &v)
	return v, err
}

func (r *RelayClient) getPublicEVEJSON(ctx context.Context, path string, out any) error {
	return r.doRequest(ctx, http.MethodGet, evePublicPath+path, nil, out, false)
}

func (r *RelayClient) FetchEVEUniverseNames(ctx context.Context, ids []int64) ([]EVEUniverseName, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, fmt.Errorf("ids must contain 1 to 1000 entries")
	}
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if err := positiveEVEID("universe id", id); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			normalized = append(normalized, id)
		}
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	var v []EVEUniverseName
	err = r.doRequest(ctx, http.MethodPost, evePublicPath+"universe/names", body, &v, false)
	return v, err
}

func (r *RelayClient) FetchEVEStatus(ctx context.Context) (EVEStatus, error) {
	var v EVEStatus
	err := r.getPublicEVEJSON(ctx, "status", &v)
	return v, err
}
func (r *RelayClient) FetchEVEMarketPrices(ctx context.Context) ([]EVEMarketPrice, error) {
	var v []EVEMarketPrice
	err := r.getPublicEVEJSON(ctx, "market/prices", &v)
	return v, err
}
func (r *RelayClient) FetchEVECorporation(ctx context.Context, corporationID int64) (EVECorporation, error) {
	var v EVECorporation
	if err := positiveEVEID("corporation id", corporationID); err != nil {
		return v, err
	}
	err := r.getPublicEVEJSON(ctx, "corporations/"+strconv.FormatInt(corporationID, 10), &v)
	return v, err
}
func (r *RelayClient) FetchEVEAlliance(ctx context.Context, allianceID int64) (EVEAlliance, error) {
	var v EVEAlliance
	if err := positiveEVEID("alliance id", allianceID); err != nil {
		return v, err
	}
	err := r.getPublicEVEJSON(ctx, "alliances/"+strconv.FormatInt(allianceID, 10), &v)
	return v, err
}
func (r *RelayClient) FetchEVEUniverseStation(ctx context.Context, stationID int64) (EVEStation, error) {
	var v EVEStation
	if err := positiveEVEID("station id", stationID); err != nil {
		return v, err
	}
	key := "station/" + strconv.FormatInt(stationID, 10)
	if cached, ok := r.entityCache.get(key); ok {
		return cached.(EVEStation), nil
	}
	err := r.getPublicEVEJSON(ctx, "universe/stations/"+strconv.FormatInt(stationID, 10), &v)
	if err == nil {
		r.entityCache.put(key, v)
	}
	return v, err
}
func (r *RelayClient) FetchEVEUniverseType(ctx context.Context, typeID int64) (EVEItemType, error) {
	var v EVEItemType
	if err := positiveEVEID("type id", typeID); err != nil {
		return v, err
	}
	key := "type/" + strconv.FormatInt(typeID, 10)
	if cached, ok := r.entityCache.get(key); ok {
		return cached.(EVEItemType), nil
	}
	err := r.getPublicEVEJSON(ctx, "universe/types/"+strconv.FormatInt(typeID, 10), &v)
	if err == nil {
		r.entityCache.put(key, v)
	}
	return v, err
}
func (r *RelayClient) FetchEVEUniverseSystem(ctx context.Context, systemID int64) (EVESolarSystem, error) {
	var v EVESolarSystem
	if err := positiveEVEID("system id", systemID); err != nil {
		return v, err
	}
	key := "system/" + strconv.FormatInt(systemID, 10)
	if cached, ok := r.entityCache.get(key); ok {
		return cached.(EVESolarSystem), nil
	}
	err := r.getPublicEVEJSON(ctx, "universe/systems/"+strconv.FormatInt(systemID, 10), &v)
	if err == nil {
		r.entityCache.put(key, v)
	}
	return v, err
}

func (r *RelayClient) FetchEVETradeHubs(ctx context.Context) ([]EVETradeHub, error) {
	var v []EVETradeHub
	err := r.getPublicEVEJSON(ctx, "market/hubs", &v)
	return v, err
}
func (r *RelayClient) FetchEVEHubComparison(ctx context.Context, typeID int64) ([]EVEHubQuote, error) {
	if err := positiveEVEID("type id", typeID); err != nil {
		return nil, err
	}
	var v []EVEHubQuote
	err := r.getPublicEVEJSON(ctx, "market/types/"+strconv.FormatInt(typeID, 10)+"/comparison", &v)
	return v, err
}
func (r *RelayClient) FetchEVETradePlans(ctx context.Context, req EVETradePlanRequest) ([]map[string]any, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var v []map[string]any
	err = r.doRequest(ctx, http.MethodPost, evePublicPath+"market/plans", body, &v, true)
	return v, err
}

func (r *RelayClient) FetchEVEMarketOrders(ctx context.Context, regionID int64, orderType string, typeID int64) ([]EVEMarketOrder, error) {
	if err := positiveEVEID("region id", regionID); err != nil {
		return nil, err
	}
	orderType = strings.ToLower(strings.TrimSpace(orderType))
	if orderType == "" {
		orderType = "all"
	}
	if orderType != "all" && orderType != "buy" && orderType != "sell" {
		return nil, fmt.Errorf("order type must be all, buy, or sell")
	}
	if typeID < 0 {
		return nil, fmt.Errorf("type id cannot be negative")
	}
	query := url.Values{"order_type": {orderType}}
	if typeID > 0 {
		query.Set("type_id", strconv.FormatInt(typeID, 10))
	}
	var v []EVEMarketOrder
	err := r.getPublicEVEJSON(ctx, "markets/"+strconv.FormatInt(regionID, 10)+"/orders?"+query.Encode(), &v)
	return v, err
}

func (r *RelayClient) FetchEVEMarketHistory(ctx context.Context, regionID, typeID int64) ([]EVEMarketHistory, error) {
	if err := positiveEVEID("region id", regionID); err != nil {
		return nil, err
	}
	if err := positiveEVEID("type id", typeID); err != nil {
		return nil, err
	}
	query := url.Values{"type_id": {strconv.FormatInt(typeID, 10)}}
	var v []EVEMarketHistory
	err := r.getPublicEVEJSON(ctx, "markets/"+strconv.FormatInt(regionID, 10)+"/history?"+query.Encode(), &v)
	return v, err
}
