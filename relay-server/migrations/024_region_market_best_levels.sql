-- Per-location top-of-book summaries for completed market batches. These rows are
-- built before a batch is promoted, so candidate reads never scan raw orders.
CREATE TABLE region_market_best_levels (
  batch_id UUID NOT NULL REFERENCES region_market_batches(id) ON DELETE CASCADE,
  region_id BIGINT NOT NULL CHECK (region_id > 0 AND region_id <= 2147483647),
  location_id BIGINT NOT NULL CHECK (location_id > 0),
  system_id BIGINT NOT NULL CHECK (system_id > 0 AND system_id <= 2147483647),
  type_id BIGINT NOT NULL CHECK (type_id > 0 AND type_id <= 2147483647),
  best_ask NUMERIC(20,2),
  ask_depth BIGINT,
  best_bid NUMERIC(20,2),
  bid_depth BIGINT,
  PRIMARY KEY (batch_id, region_id, location_id, system_id, type_id),
  CHECK ((best_ask IS NULL) = (ask_depth IS NULL)),
  CHECK ((best_bid IS NULL) = (bid_depth IS NULL)),
  CHECK (best_ask IS NULL OR (best_ask >= 0 AND ask_depth > 0)),
  CHECK (best_bid IS NULL OR (best_bid >= 0 AND bid_depth > 0)),
  CHECK (best_ask IS NOT NULL OR best_bid IS NOT NULL)
);

CREATE INDEX region_market_best_levels_ask_idx
  ON region_market_best_levels(batch_id, type_id, best_ask, region_id, location_id)
  WHERE best_ask IS NOT NULL;
CREATE INDEX region_market_best_levels_bid_idx
  ON region_market_best_levels(batch_id, type_id, best_bid DESC, region_id, location_id)
  WHERE best_bid IS NOT NULL;
