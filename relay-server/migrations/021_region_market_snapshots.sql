-- Durable, normalized full-region market snapshots. Collection writes into an
-- unpublished batch; a single transaction promotes only complete coverage and
-- retains the immediately preceding complete snapshot for safe fallback.
CREATE TABLE region_market_batches (
  id UUID PRIMARY KEY,
  region_id BIGINT NOT NULL CHECK (region_id > 0 AND region_id <= 2147483647),
  state TEXT NOT NULL CHECK (state IN ('collecting', 'complete', 'failed')),
  expected_pages INTEGER CHECK (expected_pages IS NULL OR expected_pages > 0),
  collected_pages INTEGER NOT NULL DEFAULT 0 CHECK (collected_pages >= 0),
  order_count BIGINT NOT NULL DEFAULT 0 CHECK (order_count >= 0),
  started_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  etag TEXT,
  failure TEXT,
  CHECK ((state = 'complete') = (completed_at IS NOT NULL)),
  CHECK (state <> 'complete' OR (expected_pages IS NOT NULL AND collected_pages = expected_pages)),
  CHECK (length(COALESCE(failure, '')) <= 1024)
);

CREATE INDEX region_market_batches_region_started_idx
  ON region_market_batches(region_id, started_at DESC);

CREATE TABLE region_market_batch_pages (
  batch_id UUID NOT NULL REFERENCES region_market_batches(id) ON DELETE CASCADE,
  page_number INTEGER NOT NULL CHECK (page_number > 0),
  order_count INTEGER NOT NULL CHECK (order_count >= 0 AND order_count <= 10000),
  PRIMARY KEY (batch_id, page_number)
);

CREATE TABLE region_market_orders (
  batch_id UUID NOT NULL REFERENCES region_market_batches(id) ON DELETE CASCADE,
  order_id BIGINT NOT NULL CHECK (order_id > 0),
  region_id BIGINT NOT NULL CHECK (region_id > 0 AND region_id <= 2147483647),
  type_id BIGINT NOT NULL CHECK (type_id > 0 AND type_id <= 2147483647),
  location_id BIGINT NOT NULL CHECK (location_id > 0),
  system_id BIGINT NOT NULL CHECK (system_id > 0 AND system_id <= 2147483647),
  is_buy_order BOOLEAN NOT NULL,
  price NUMERIC(20,2) NOT NULL CHECK (price >= 0),
  volume_total BIGINT NOT NULL CHECK (volume_total >= 0),
  volume_remain BIGINT NOT NULL CHECK (volume_remain >= 0 AND volume_remain <= volume_total),
  min_volume BIGINT NOT NULL CHECK (min_volume > 0),
  order_range TEXT NOT NULL CHECK (length(order_range) BETWEEN 1 AND 32),
  duration INTEGER NOT NULL CHECK (duration > 0),
  issued TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (batch_id, order_id)
);

CREATE INDEX region_market_orders_snapshot_lookup_idx
  ON region_market_orders(batch_id, type_id, is_buy_order, price);
CREATE INDEX region_market_orders_location_idx
  ON region_market_orders(batch_id, location_id, type_id);

CREATE TABLE region_market_snapshots (
  region_id BIGINT PRIMARY KEY CHECK (region_id > 0 AND region_id <= 2147483647),
  current_batch_id UUID NOT NULL REFERENCES region_market_batches(id) ON DELETE RESTRICT,
  previous_batch_id UUID REFERENCES region_market_batches(id) ON DELETE RESTRICT,
  published_at TIMESTAMPTZ NOT NULL,
  CHECK (previous_batch_id IS NULL OR previous_batch_id <> current_batch_id)
);
