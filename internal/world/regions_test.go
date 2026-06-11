package world

import "testing"

// allRegionIDs is every Region* constant declared in world.go. Tests
// iterate it to keep the kind and cost tables total.
var allRegionIDs = []int64{
	RegionPlateau, RegionMountain, RegionCradle, RegionBrine,
	RegionEastSea, RegionUnknown, RegionDrowned, RegionDoab,
	RegionCliff, RegionFoothill, RegionGlacier, RegionAgraria,
	RegionAgrariaUpland, RegionLake, RegionForest, RegionTundra,
	RegionMarsh, RegionSeat, RegionMarch, RegionHeadwater,
	RegionOuthold, RegionReach, RegionPass, RegionDragonDen,
	RegionDrakeNest, RegionWyvernRookery,
}

func TestRegionKind_Total(t *testing.T) {
	if len(kindByRegionID) != len(allRegionIDs) {
		t.Errorf("kindByRegionID has %d entries, want %d", len(kindByRegionID), len(allRegionIDs))
	}
	seen := make(map[string]int64)
	for _, id := range allRegionIDs {
		kind := RegionKind(id)
		if kind == "" {
			t.Errorf("RegionKind(%d) is empty — every region ID needs a kind", id)
			continue
		}
		if other, dup := seen[kind]; dup {
			t.Errorf("kind %q mapped from both region %d and %d", kind, other, id)
		}
		seen[kind] = id
		// Round trip through the inverted map.
		if back := regionIDByKind[kind]; back != id {
			t.Errorf("regionIDByKind[%q] = %d, want %d", kind, back, id)
		}
	}
	if RegionKind(999) != "" {
		t.Errorf("RegionKind(unknown id) = %q, want empty", RegionKind(999))
	}
}

func TestTravelCost_FacadeMatchesCanonicalTable(t *testing.T) {
	for _, id := range allRegionIDs {
		kind := RegionKind(id)
		if got, want := TravelCost(kind), travelCostFor(id); got != want {
			t.Errorf("TravelCost(%q) = %d, want travelCostFor(%d) = %d", kind, got, id, want)
		}
	}
	if got := TravelCost("not_a_kind"); got != -1 {
		t.Errorf("TravelCost(unknown kind) = %d, want -1", got)
	}
}

func TestRoadBuildCost_Overrides(t *testing.T) {
	// Roads can't be built over the plateau or through lairs even
	// though expeditions can slog across them.
	for _, id := range []int64{RegionPlateau, RegionDragonDen, RegionDrakeNest, RegionWyvernRookery} {
		if got := roadBuildCost(id); got != -1 {
			t.Errorf("roadBuildCost(%s) = %d, want -1", RegionKind(id), got)
		}
		if travelCostFor(id) < 0 {
			t.Errorf("travelCostFor(%s) = %d — expected passable for expeditions", RegionKind(id), travelCostFor(id))
		}
	}
	// Everything else matches the canonical table.
	for _, id := range allRegionIDs {
		switch id {
		case RegionPlateau, RegionDragonDen, RegionDrakeNest, RegionWyvernRookery:
			continue
		}
		if got, want := roadBuildCost(id), travelCostFor(id); got != want {
			t.Errorf("roadBuildCost(%s) = %d, want %d", RegionKind(id), got, want)
		}
	}
}
