package world

// travelCostFor is the canonical terrain cost table for overland foot
// travel, keyed by region ID. Returns -1 if the terrain is impassable
// (mountain, cliff, sea, glacier, lake, drowned, unknown). River
// presence is handled by the caller (rivers cost 1).
func travelCostFor(id int64) int {
	switch id {
	case RegionSeat, RegionMarch, RegionHeadwater, RegionOuthold, RegionReach, RegionCapital:
		return 2
	case RegionPass:
		return 3
	case RegionCradle, RegionForest, RegionTundra, RegionAgraria, RegionAgrariaUpland:
		return 4
	case RegionRuin:
		return 4 // broken walls on open ground — walkable, nothing more
	case RegionFoothill:
		return 5
	case RegionDoab:
		return 6
	case RegionMarsh:
		return 8
	case RegionPlateau:
		return 15
	case RegionDragonDen, RegionDrakeNest, RegionWyvernRookery:
		return 25
	}
	return -1
}

// TravelCost is the kind-string façade over travelCostFor for callers
// that hold DB rows (the regions table stores kinds, not our IDs).
func TravelCost(kind string) int {
	id, ok := regionIDByKind[kind]
	if !ok {
		return -1
	}
	return travelCostFor(id)
}

// roadBuildCost is the road-construction variant of the travel table:
// an expedition can slog across the plateau or sneak past a lair, but
// nobody builds a trade road there.
func roadBuildCost(id int64) int {
	switch id {
	case RegionPlateau, RegionDragonDen, RegionDrakeNest, RegionWyvernRookery:
		return -1
	}
	return travelCostFor(id)
}
