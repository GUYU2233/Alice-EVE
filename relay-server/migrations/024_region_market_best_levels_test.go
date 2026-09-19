package migrations

import (
	"strings"
	"testing"
)

func TestBestLevelsMigrationProperties(t *testing.T) {
	contents, err := SQLFiles.ReadFile("024_region_market_best_levels.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(contents)
	for _, want := range []string{
		"CREATE TABLE region_market_best_levels",
		"REFERENCES region_market_batches(id) ON DELETE CASCADE",
		"PRIMARY KEY (batch_id, region_id, location_id, system_id, type_id)",
		"best_ask NUMERIC(20,2)", "ask_depth BIGINT",
		"best_bid NUMERIC(20,2)", "bid_depth BIGINT",
		"region_market_best_levels_ask_idx", "region_market_best_levels_bid_idx",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}
