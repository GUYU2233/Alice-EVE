package marketdata

import (
	"context"
	"errors"
)

func (r *PostgresRepository) SearchCandidates(ctx context.Context, q CandidateSearch) (CandidatePage, error) {
	if err := q.normalize(); err != nil {
		return CandidatePage{}, err
	}
	rows, err := r.pool.Query(ctx, candidateSearchSQL, q.SourceRegionIDs, q.DestinationRegionIDs, q.DestinationScope, q.PerTypeLocations, q.Budget, q.CargoM3, q.SalesTaxRate, q.BrokerRate, q.MinProfit, q.MinProfitRate, q.Limit+1, q.Offset)
	if err != nil {
		return CandidatePage{}, errors.Join(errCandidateQuery, err)
	}
	defer rows.Close()
	out := CandidatePage{Items: make([]TradeCandidate, 0, q.Limit), Limit: q.Limit, Offset: q.Offset}
	for rows.Next() {
		var c TradeCandidate
		if err := rows.Scan(&c.TypeID, &c.SourceRegionID, &c.SourceLocationID, &c.SourceSystemID, &c.DestinationRegionID, &c.DestinationLocationID, &c.DestinationSystemID, &c.BuyPrice, &c.SellPrice, &c.ItemVolumeM3, &c.Quantity, &c.Capital, &c.GrossProfit, &c.Fees, &c.NetProfit, &c.ProfitRate, &c.CargoUsedM3, &c.SourceSnapshotAt, &c.DestinationSnapshotAt); err != nil {
			return CandidatePage{}, err
		}
		c.RouteSafetyStatus = "pending"
		if q.IncludeDepth {
			c.AskLevels, err = r.depthLevels(ctx, c.SourceRegionID, c.SourceLocationID, c.TypeID, false)
			if err != nil {
				return CandidatePage{}, err
			}
			c.BidLevels, err = r.depthLevels(ctx, c.DestinationRegionID, c.DestinationLocationID, c.TypeID, true)
			if err != nil {
				return CandidatePage{}, err
			}
		}
		if len(out.Items) == q.Limit {
			out.HasMore = true
			continue
		}
		out.Items = append(out.Items, c)
	}
	if err := rows.Err(); err != nil {
		return CandidatePage{}, err
	}
	return out, nil
}

func (r *PostgresRepository) depthLevels(ctx context.Context, regionID, locationID, typeID int64, buy bool) ([]DepthLevel, error) {
	rows, err := r.pool.Query(ctx, depthLevelsSQL, regionID, locationID, typeID, buy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DepthLevel, 0, 32)
	for rows.Next() {
		var x DepthLevel
		if err := rows.Scan(&x.Price, &x.Volume, &x.CumulativeVolume, &x.MinimumVolume); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

const depthLevelsSQL = `
SELECT d.price,d.volume,d.cumulative_volume,d.minimum_volume
FROM region_market_snapshots s
JOIN region_market_order_depth_levels d ON d.batch_id=s.current_batch_id AND d.region_id=s.region_id
WHERE s.region_id=$1 AND d.location_id=$2 AND d.type_id=$3 AND d.is_buy_order=$4
ORDER BY d.level_no LIMIT 32`

// Candidate reads are bounded by the precomputed top-of-book table. Raw orders
// are touched only once, while publishing an unpublished batch.
const candidateSearchSQL = `
WITH source_selected AS MATERIALIZED (
 SELECT region_id,current_batch_id,published_at FROM region_market_snapshots WHERE region_id=ANY($1::bigint[])
), destination_selected AS MATERIALIZED (
 SELECT region_id,current_batch_id,published_at FROM region_market_snapshots
 WHERE $3='all_collected_regions' OR region_id=ANY($2::bigint[])
), type_volume AS MATERIALIZED (
 SELECT t.type_id,COALESCE(NULLIF(t.packaged_volume,0),t.volume) volume_m3
 FROM eve_types t JOIN eve_sde_builds b ON b.id=t.build_id AND b.state='active'
 WHERE t.published AND COALESCE(NULLIF(t.packaged_volume,0),t.volume)>0
), asks AS MATERIALIZED (
 SELECT l.region_id,l.location_id,l.system_id,l.type_id,l.best_ask,l.ask_depth,s.published_at,v.volume_m3,
  ROW_NUMBER() OVER(PARTITION BY l.type_id ORDER BY l.best_ask,l.ask_depth DESC,l.region_id,l.location_id) rn
 FROM source_selected s JOIN region_market_best_levels l ON l.batch_id=s.current_batch_id AND l.region_id=s.region_id
 JOIN type_volume v ON v.type_id=l.type_id WHERE l.best_ask>0
), bids AS MATERIALIZED (
 SELECT l.region_id,l.location_id,l.system_id,l.type_id,l.best_bid,l.bid_depth,s.published_at,
  ROW_NUMBER() OVER(PARTITION BY l.type_id ORDER BY l.best_bid DESC,l.bid_depth DESC,l.region_id,l.location_id) rn
 FROM destination_selected s JOIN region_market_best_levels l ON l.batch_id=s.current_batch_id AND l.region_id=s.region_id
 WHERE l.best_bid IS NOT NULL
), sized AS (
 SELECT a.type_id,a.region_id,a.location_id,a.system_id,a.best_ask price,a.ask_depth depth,a.published_at,a.volume_m3,
  b.region_id dst_region,b.location_id dst_location,b.system_id dst_system,b.best_bid sell_price,b.bid_depth sell_depth,b.published_at dst_at,
  LEAST(a.ask_depth,b.bid_depth,FLOOR($5/a.best_ask)::bigint,FLOOR($6/a.volume_m3)::bigint) qty
 FROM asks a JOIN bids b USING(type_id) WHERE a.rn<=$4 AND b.rn<=$4
 AND (a.region_id,a.location_id)<>(b.region_id,b.location_id) AND b.best_bid>a.best_ask
), valued AS (
 SELECT *,price*qty capital,(sell_price-price)*qty gross,(sell_price*qty*$7+price*qty*$8) fees FROM sized WHERE qty>0
)
SELECT type_id,region_id,location_id,system_id,dst_region,dst_location,dst_system,price,sell_price,volume_m3,qty,
 capital,gross,fees,gross-fees,(gross-fees)/capital,volume_m3*qty,published_at,dst_at
FROM valued WHERE gross-fees >= $9 AND (gross-fees)/capital >= $10
ORDER BY gross-fees DESC,volume_m3*qty DESC,type_id LIMIT $11 OFFSET $12`

var _ CandidateRepository = (*PostgresRepository)(nil)
