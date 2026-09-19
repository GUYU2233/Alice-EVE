// Package esipublic refreshes account-independent ESI data into the public cache.
// It is deliberately separate from esisync, which owns character-scoped work.
package esipublic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"relay-server/internal/esi"
	"relay-server/internal/esidata"
)

const MaxUniverseID int64 = 2147483647

const (
	KindStatus          = "status"
	KindMarketPrices    = "market-prices"
	KindUniverseType    = "universe-type"
	KindUniverseSystem  = "universe-system"
	KindUniverseStation = "universe-station"
	KindUniverseNames   = "universe-names"
	KindCorporation     = "corporation"
	KindAlliance        = "alliance"
	KindRegionOrders    = "region-orders"
	KindRegionHistory   = "region-history"
	CurrentKey          = "current"
)

type Result struct {
	Entry esidata.PublicData
	Stale bool
}

type Service struct {
	gateway *esi.Gateway
	repo    esidata.Repository
	now     func() time.Time
	mu      sync.Mutex
}

func New(gateway *esi.Gateway, repo esidata.Repository) (*Service, error) {
	if gateway == nil || repo == nil {
		return nil, errors.New("ESI gateway and data repository are required")
	}
	return &Service{gateway: gateway, repo: repo, now: time.Now}, nil
}

func (s *Service) Gateway() *esi.Gateway { return s.gateway }

func (s *Service) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Service) Status(ctx context.Context) (Result, error) {
	return s.load(ctx, KindStatus, CurrentKey, 30*time.Second, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.Status(ctx) })
}
func (s *Service) MarketPrices(ctx context.Context) (Result, error) {
	return s.load(ctx, KindMarketPrices, CurrentKey, time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.MarketPrices(ctx) })
}
func (s *Service) UniverseNames(ctx context.Context, ids []int64) (Result, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return Result{}, fmt.Errorf("IDs must contain 1 to 1000 entries")
	}
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if err := boundedID("universe", id); err != nil {
			return Result{}, err
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			normalized = append(normalized, id)
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, len(normalized))
	for i, id := range normalized {
		parts[i] = strconv.FormatInt(id, 10)
	}
	key := strings.Join(parts, ",")
	return s.load(ctx, KindUniverseNames, key, 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.UniverseNames(ctx, normalized) })
}

func (s *Service) UniverseType(ctx context.Context, id int64) (Result, error) {
	if err := boundedID("type", id); err != nil {
		return Result{}, err
	}
	return s.load(ctx, KindUniverseType, strconv.FormatInt(id, 10), 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.UniverseType(ctx, id) })
}
func (s *Service) UniverseSystem(ctx context.Context, id int64) (Result, error) {
	if err := boundedID("system", id); err != nil {
		return Result{}, err
	}
	return s.load(ctx, KindUniverseSystem, strconv.FormatInt(id, 10), 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.UniverseSystem(ctx, id) })
}
func (s *Service) Corporation(ctx context.Context, id int64) (Result, error) {
	if err := boundedID("corporation", id); err != nil {
		return Result{}, err
	}
	return s.load(ctx, KindCorporation, strconv.FormatInt(id, 10), 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.Corporation(ctx, id) })
}
func (s *Service) Alliance(ctx context.Context, id int64) (Result, error) {
	if err := boundedID("alliance", id); err != nil {
		return Result{}, err
	}
	return s.load(ctx, KindAlliance, strconv.FormatInt(id, 10), 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.Alliance(ctx, id) })
}
func (s *Service) UniverseStation(ctx context.Context, id int64) (Result, error) {
	if err := boundedID("station", id); err != nil {
		return Result{}, err
	}
	return s.load(ctx, KindUniverseStation, strconv.FormatInt(id, 10), 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.UniverseStation(ctx, id) })
}
func (s *Service) RegionOrders(ctx context.Context, regionID int64, orderType string, typeID int64) (Result, error) {
	if err := boundedID("region", regionID); err != nil {
		return Result{}, err
	}
	orderType = strings.ToLower(strings.TrimSpace(orderType))
	if orderType == "" {
		orderType = "all"
	}
	if orderType != "all" && orderType != "buy" && orderType != "sell" {
		return Result{}, fmt.Errorf("invalid order type")
	}
	if typeID < 0 || typeID > MaxUniverseID {
		return Result{}, fmt.Errorf("type ID out of range")
	}
	key := fmt.Sprintf("%d:%s:%d", regionID, orderType, typeID)
	return s.load(ctx, KindRegionOrders, key, 5*time.Minute, func(ctx context.Context) (any, esi.Response, error) {
		return s.gateway.RegionOrders(ctx, regionID, orderType, typeID)
	})
}
func (s *Service) RegionHistory(ctx context.Context, regionID, typeID int64) (Result, error) {
	if err := boundedID("region", regionID); err != nil {
		return Result{}, err
	}
	if err := boundedID("type", typeID); err != nil {
		return Result{}, err
	}
	key := fmt.Sprintf("%d:%d", regionID, typeID)
	return s.load(ctx, KindRegionHistory, key, 24*time.Hour, func(ctx context.Context) (any, esi.Response, error) {
		return s.gateway.RegionHistory(ctx, regionID, typeID)
	})
}

// RefreshStatus and RefreshMarketPrices are suitable for a small periodic public
// updater. The remaining domains are intentionally refreshed on demand.
func (s *Service) RefreshStatus(ctx context.Context) (Result, error) {
	return s.refresh(ctx, KindStatus, CurrentKey, 30*time.Second, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.Status(ctx) })
}
func (s *Service) RefreshMarketPrices(ctx context.Context) (Result, error) {
	return s.refresh(ctx, KindMarketPrices, CurrentKey, time.Hour, func(ctx context.Context) (any, esi.Response, error) { return s.gateway.MarketPrices(ctx) })
}

func boundedID(name string, id int64) error {
	if id <= 0 || id > MaxUniverseID {
		return fmt.Errorf("%s ID out of range", name)
	}
	return nil
}

type fetchFunc func(context.Context) (any, esi.Response, error)

func (s *Service) load(ctx context.Context, kind, key string, fallback time.Duration, fetch fetchFunc) (Result, error) {
	now := s.now()
	cached, err := s.repo.GetPublicData(ctx, kind, key)
	if err == nil && cached.ExpiresAt.After(now) {
		return Result{Entry: cached}, nil
	}
	if err != nil && !errors.Is(err, esidata.ErrNotFound) {
		return Result{}, err
	}
	return s.refreshWithStale(ctx, kind, key, fallback, fetch, cached, err == nil)
}
func (s *Service) refresh(ctx context.Context, kind, key string, fallback time.Duration, fetch fetchFunc) (Result, error) {
	cached, err := s.repo.GetPublicData(ctx, kind, key)
	return s.refreshWithStale(ctx, kind, key, fallback, fetch, cached, err == nil)
}
func (s *Service) refreshWithStale(ctx context.Context, kind, key string, fallback time.Duration, fetch fetchFunc, cached esidata.PublicData, hasCached bool) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, meta, err := fetch(ctx)
	if err != nil {
		if hasCached {
			return Result{Entry: cached, Stale: true}, nil
		}
		return Result{}, err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		if hasCached {
			return Result{Entry: cached, Stale: true}, nil
		}
		return Result{}, err
	}
	now := s.now()
	expires := meta.ExpiresAt
	if expires.IsZero() || expires.Before(now) {
		expires = now.Add(fallback)
	}
	etag := meta.ETag
	if etag == "" && hasCached {
		etag = cached.ETag
	}
	entry := esidata.PublicData{Kind: kind, CacheKey: key, Payload: payload, FetchedAt: now, ExpiresAt: expires, ETag: etag}
	if err := s.repo.UpsertPublicData(ctx, entry); err != nil {
		return Result{}, err
	}
	return Result{Entry: entry}, nil
}
