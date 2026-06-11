package world

import "math/rand"

// Generate produces a deterministic world from the given seed and a
// moment in geological time (kya = kiloyears before present).
//
// The pipeline is climate-driven and time-driven: a single bedrock
// model (zones + elevations) is built once from the seed and is
// stable across all kya — geology doesn't move on these timescales.
// Climate (sea level, mean temp delta) at the given kya is then
// applied per cell to derive whether the cell shows up as land,
// sea, or glacier. As kya scrubs from 205 toward 0, the ice retreats
// smoothly and Agraria submerges — both as consequences of the
// climate cycle, not hardcoded snapshots.
//
// Each stage below is a named transform over the World; ordering
// matters (later stages read the RegionIDs earlier stages wrote) and
// every stage must be deterministic — the snapshot tests pin the
// combined output.
func Generate(seed int64, kya int) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock := generateBedrock(rng)

	w := World{
		Seed:      seed,
		Kya:       kya,
		Era:       EraForKya(kya),
		LatTop:    DefaultLatTop,
		LatBottom: DefaultLatBottom,
		Orbital:   OrbitalAt(kya),
		Climate:   ClimateAt(kya),
	}

	w.classifyRegions(bedrock)

	// Rivers grow head-to-mouth as climate warms. Threshold is uniform
	// (it just identifies the river network topology); the maximum
	// length each river extends from its headwater scales with glacial
	// index — at the glacial peak, length=0 (no rivers; water locked in
	// ice). Lakes are a side-product of the same flow pass.
	var lakes []LakeCell
	w.RiverInfo, w.Rivers, lakes = flowRivers(bedrock,
		riverThreshold(),
		riverMaxLenFor(w.Climate.GlacialIndex))
	w.nameRivers()
	w.computeDrainage(bedrock)

	w.applyLakes(lakes)
	w.refineBiomes()

	w.placeSeats()
	w.placeReaches()
	w.placeOutholds()
	w.placeLairs()
	w.applyDragonPressure()

	w.findPasses()
	w.buildRoads()
	w.markMarshes()
	w.nameLakes(lakes)

	// Polity layer — runs last because it reads everything: rivers
	// (navigability = control), roads, seats, dragon pressure. The
	// political map is therefore climate-coupled: at the LGM there
	// are no rivers, no Tributaries, no capital — and no crown.
	capitalIdx := w.chooseCapital()
	w.computeAllegiance(capitalIdx)
	w.formRealms(capitalIdx)
	w.claimTerritory()
	return w
}

// classifyRegions runs the climate→surface mapper over the bedrock and
// fills w.Regions with every cell that maps to a region.
func (w *World) classifyRegions(bedrock [][]BedrockCell) {
	for y := 0; y < Height; y++ {
		lat := Latitude(y, w.LatTop, w.LatBottom)
		for x := 0; x < Width; x++ {
			b := bedrock[y][x]
			rid := classify(b, lat, w.Climate)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID:  rid,
					X:         int64(x),
					Y:         int64(y),
					Elevation: b.Elevation,
				})
			}
		}
	}
}

// classify is the climate→surface mapper. Order of precedence:
//  1. Agraria shelf gets a "is exposed?" check first — when its
//     elevation is at or above sea level, it always reads as Agraria,
//     regardless of temperature. (Lore: temperate microclimate; the
//     Coastals lived there during glacial peaks, so it can't be ice.)
//  2. Glaciation, where the zone allows it. Glacier outranks
//     submerged-water — a frozen sea surface reads as glacier (ice
//     shelf), not sea.
//  3. Submerged water, mapped to whichever sea/basin the zone is in.
//  4. Otherwise the zone's exposed-land identity.
func classify(b BedrockCell, lat float64, climate ClimateState) int64 {
	seaLevel := climate.SeaLevelDelta

	// Shelf cells: when exposed they always read as Agraria (their
	// lore identity) regardless of temperature; when submerged they
	// stay as Brine (no "sea ice" intermediate — keeps the
	// emerge/submerge transition visually clean).
	if b.Zone == BZAgrariaShelf {
		if b.Elevation >= seaLevel {
			return RegionAgraria
		}
		return RegionBrine
	}
	if b.Zone == BZAgrariaUpland {
		if b.Elevation >= seaLevel {
			return RegionAgrariaUpland
		}
		return RegionBrine
	}

	if canGlaciate(b.Zone) {
		if Temperature(lat, b.Elevation, climate) < glacierThreshold {
			return RegionGlacier
		}
	}

	// Note: cliff zone classification happens in bedrockZone now (it's a
	// bedrock property, not a climate one). Code retained here in case
	// future climate effects need to know cliff vs mountain.
	if b.Elevation < seaLevel {
		switch b.Zone {
		case BZBrineDeep, BZAgrariaShelf, BZAgrariaUpland:
			return RegionBrine
		case BZEastBasin:
			return RegionEastSea
		default:
			// land zones aren't normally below sea level
			return RegionEastSea
		}
	}

	switch b.Zone {
	case BZPlateau:
		return RegionPlateau
	case BZMountain:
		return RegionMountain
	case BZCliff:
		return RegionCliff
	case BZFoothill:
		return RegionFoothill
	case BZDoab:
		return RegionDoab
	case BZCradle:
		return RegionCradle
	case BZAgrariaShelf:
		return RegionAgraria
	case BZAgrariaUpland:
		return RegionAgrariaUpland
	case BZEastBasin:
		// Exposed (e.g., extreme low-stand) — reads as cradle-ish land.
		return RegionCradle
	case BZBrineDeep:
		// Should not normally happen; deep basin shouldn't be exposed.
		return RegionUnknown
	}
	return 0
}
