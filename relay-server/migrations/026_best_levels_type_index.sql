-- Candidate search joins active snapshots by batch and then market metadata by type.
CREATE INDEX region_market_best_levels_type_batch_idx
  ON region_market_best_levels(type_id,batch_id)
  INCLUDE(region_id,location_id,system_id,best_ask,ask_depth,best_bid,bid_depth);
CREATE INDEX eve_types_active_lookup_idx
  ON eve_types(build_id,type_id)
  INCLUDE(published,packaged_volume,volume);
