package world

// kindByRegionID mirrors the `regions` table seeded by the migrations
// and is the Go-side source of truth for RegionID ↔ kind string. The
// kind strings are what the DB layer and renderer key on; keep this in
// sync with the migration that introduces a new region.
var kindByRegionID = map[int64]string{
	RegionPlateau:       "plateau",
	RegionMountain:      "mountain",
	RegionCradle:        "cradle",
	RegionBrine:         "sea_brine",
	RegionEastSea:       "sea_eastern",
	RegionUnknown:       "unknown",
	RegionDrowned:       "drowned",
	RegionDoab:          "doab",
	RegionCliff:         "cliff",
	RegionFoothill:      "foothill",
	RegionGlacier:       "glacier",
	RegionAgraria:       "agraria",
	RegionAgrariaUpland: "agraria_upland",
	RegionLake:          "lake",
	RegionForest:        "forest",
	RegionTundra:        "tundra",
	RegionMarsh:         "marsh",
	RegionSeat:          "seat",
	RegionMarch:         "march",
	RegionHeadwater:     "headwater",
	RegionOuthold:       "outhold",
	RegionReach:         "reach",
	RegionPass:          "pass",
	RegionDragonDen:     "den",
	RegionDrakeNest:     "nest",
	RegionWyvernRookery: "rookery",
	RegionCapital:       "capital",
	RegionRuin:          "ruin",
	RegionVolcano:       "volcano",
	RegionLava:          "lava",
}

var regionIDByKind = func() map[string]int64 {
	m := make(map[string]int64, len(kindByRegionID))
	for id, kind := range kindByRegionID {
		m[kind] = id
	}
	return m
}()

// RegionKind returns the kind string for a region ID ("" if unknown).
func RegionKind(id int64) string { return kindByRegionID[id] }
