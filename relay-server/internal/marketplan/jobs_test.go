package marketplan

import (
	"errors"
	"testing"
)

func TestNormalizeDefaultsAndStableHash(t *testing.T) {
	a := CreateRequest{Mode: "BASKET", SourceRegionIDs: []int64{10000043, 10000002}, Constraints: Constraints{Budget: 1000, BudgetReserve: 100, CargoM3: 50, MinSecurity: .5, MaxJumps: 15}}
	h, e := Normalize(&a)
	if e != nil {
		t.Fatal(e)
	}
	if a.DestinationScope != "all_collected_regions" || a.DestinationRegionIDs == nil || a.Constraints.TargetLoadFactor != .9 || len(h) != 64 {
		t.Fatalf("%+v hash=%s", a, h)
	}
	b := CreateRequest{Mode: "basket", SourceRegionIDs: []int64{10000002, 10000043}, Constraints: Constraints{Budget: 1000, BudgetReserve: 100, CargoM3: 50, MinSecurity: .5, MaxJumps: 15, TargetLoadFactor: .9}}
	h2, e := Normalize(&b)
	if e != nil || h != h2 {
		t.Fatalf("hash mismatch %s %s %v", h, h2, e)
	}
}
func TestNormalizeRejectsUnsafeJobs(t *testing.T) {
	cases := []CreateRequest{{Mode: "basket", Constraints: Constraints{Budget: 1, CargoM3: 1}}, {Mode: "x", SourceRegionIDs: []int64{1}, Constraints: Constraints{Budget: 1, CargoM3: 1}}, {Mode: "single", SourceRegionIDs: []int64{1}, DestinationScope: "selected_regions", Constraints: Constraints{Budget: 1, CargoM3: 1}}, {Mode: "chain", SourceRegionIDs: []int64{1}, Constraints: Constraints{Budget: 1, BudgetReserve: 1, CargoM3: 1}}}
	for i := range cases {
		if !errors.Is(func() error { _, e := Normalize(&cases[i]); return e }(), ErrInvalidInput) {
			t.Fatalf("case %d accepted", i)
		}
	}
}
