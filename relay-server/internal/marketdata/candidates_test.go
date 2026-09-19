package marketdata

import (
	"errors"
	"testing"
)

func TestCandidateSearchNormalize(t *testing.T) {
	q := CandidateSearch{RegionIDs: []int64{10000002, 10000043}, Budget: 1_000_000, CargoM3: 500}
	if err := q.normalize(); err != nil {
		t.Fatal(err)
	}
	if q.Limit != DefaultCandidateLimit {
		t.Fatalf("limit=%d", q.Limit)
	}
}

func TestCandidateSearchSeparatesSourceAndGlobalDestinations(t *testing.T) {
	q := CandidateSearch{SourceRegionIDs: []int64{10000002}, DestinationScope: "all_collected_regions", Budget: 1, CargoM3: 1}
	if err := q.normalize(); err != nil {
		t.Fatal(err)
	}
	if q.PerTypeLocations != 8 || len(q.SourceRegionIDs) != 1 {
		t.Fatalf("normalized=%+v", q)
	}
	q = CandidateSearch{SourceRegionIDs: []int64{10000002}, DestinationScope: "selected_regions", Budget: 1, CargoM3: 1}
	if !errors.Is(q.normalize(), ErrInvalidInput) {
		t.Fatal("selected destination scope requires destination regions")
	}
}

func TestCandidateSearchRejectsUnsafeBounds(t *testing.T) {
	cases := []CandidateSearch{
		{RegionIDs: nil, Budget: 1, CargoM3: 1},
		{RegionIDs: []int64{1}, Budget: 0, CargoM3: 1},
		{RegionIDs: []int64{1}, Budget: 1, CargoM3: 0},
		{RegionIDs: []int64{1, 1}, Budget: 1, CargoM3: 1},
		{RegionIDs: []int64{1}, Budget: 1, CargoM3: 1, Limit: MaxCandidateLimit + 1},
		{RegionIDs: []int64{1}, Budget: 1, CargoM3: 1, Offset: MaxCandidateOffset + 1},
		{RegionIDs: []int64{1}, Budget: 1, CargoM3: 1, SalesTaxRate: 1},
	}
	for i := range cases {
		if err := cases[i].normalize(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestCandidateSQLIsBoundedAndUsesCurrentSummaries(t *testing.T) {
	for _, want := range []string{"source_selected", "destination_selected", "all_collected_regions", "region_market_snapshots", "current_batch_id", "region_market_best_levels", "LIMIT $11 OFFSET $12", "LEAST(a.ask_depth,b.bid_depth", "a.rn<=$4", "b.rn<=$4", "eve_types", "b.state='active'", "COALESCE(NULLIF(t.packaged_volume,0),t.volume)", "best_ask>0"} {
		if !contains(candidateSearchSQL, want) {
			t.Errorf("SQL missing %q", want)
		}
	}
	for _, forbidden := range []string{"public_data_cache", "region_market_orders"} {
		if contains(candidateSearchSQL, forbidden) {
			t.Errorf("candidate query must not depend on %s", forbidden)
		}
	}
}

func TestPublishBuildsSummaryBeforeSwitchingCurrent(t *testing.T) {
	if !contains(buildBestLevelsSQL, "region_market_orders") || !contains(buildBestLevelsSQL, "SUM(volume_remain)") || !contains(buildBestLevelsSQL, "ROW_NUMBER()") {
		t.Error("publish summary must aggregate raw order depth and rank prices")
	}
	if contains(buildBestLevelsSQL, "region_market_snapshots") {
		t.Error("summary build must be scoped to the unpublished batch, not current snapshot")
	}
}

func TestDepthQueryUsesCurrentSnapshotAndIsBounded(t *testing.T) {
	for _, want := range []string{"region_market_snapshots", "current_batch_id", "region_market_order_depth_levels", "level_no", "LIMIT 32"} {
		if !contains(depthLevelsSQL, want) {
			t.Errorf("depth query missing %q", want)
		}
	}
}

func TestPublishBuildsBoundedMultiLevelDepth(t *testing.T) {
	for _, want := range []string{"region_market_order_depth_levels", "SUM(volume_remain)", "MIN(GREATEST(min_volume,1))", "ROW_NUMBER()", "level_no<=32", "cumulative_volume"} {
		if !contains(buildDepthLevelsSQL, want) {
			t.Errorf("depth SQL missing %q", want)
		}
	}
	if contains(buildDepthLevelsSQL, "region_market_snapshots") {
		t.Error("depth build must be scoped to the unpublished batch")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
