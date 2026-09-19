-- Bounded multi-level order-book summaries used by executable trade planning.
-- Each row is an aggregated price level, not an individual market order.
CREATE TABLE region_market_order_depth_levels (
  batch_id UUID NOT NULL REFERENCES region_market_batches(id) ON DELETE CASCADE,
  region_id BIGINT NOT NULL CHECK (region_id > 0 AND region_id <= 2147483647),
  location_id BIGINT NOT NULL CHECK (location_id > 0),
  system_id BIGINT NOT NULL CHECK (system_id > 0 AND system_id <= 2147483647),
  type_id BIGINT NOT NULL CHECK (type_id > 0 AND type_id <= 2147483647),
  is_buy_order BOOLEAN NOT NULL,
  level_no SMALLINT NOT NULL CHECK (level_no BETWEEN 1 AND 32),
  price NUMERIC(20,2) NOT NULL CHECK (price >= 0),
  volume BIGINT NOT NULL CHECK (volume > 0),
  cumulative_volume BIGINT NOT NULL CHECK (cumulative_volume >= volume),
  minimum_volume BIGINT NOT NULL CHECK (minimum_volume > 0),
  source_order_count INTEGER NOT NULL CHECK (source_order_count > 0),
  PRIMARY KEY (batch_id, region_id, location_id, system_id, type_id, is_buy_order, level_no)
);

CREATE INDEX region_market_depth_ask_lookup_idx
  ON region_market_order_depth_levels(batch_id, type_id, location_id, level_no)
  INCLUDE (region_id, system_id, price, volume, cumulative_volume, minimum_volume)
  WHERE NOT is_buy_order;

CREATE INDEX region_market_depth_bid_lookup_idx
  ON region_market_order_depth_levels(batch_id, type_id, location_id, level_no)
  INCLUDE (region_id, system_id, price, volume, cumulative_volume, minimum_volume)
  WHERE is_buy_order;
