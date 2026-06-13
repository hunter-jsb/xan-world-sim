package world

import "math/rand"

// Generate produces a deterministic world from the given seed and a
// moment in geological time (kya = kiloyears before present).
//
// The pipeline is climate-driven and time-driven twice over. The
// bedrock's structural frame (zones) is built once from the seed and
// is stable across all kya, but the rock itself has a history: from
// that frame, geology.go integrates uplift, volcanism, glacial work,
// isostasy, and erosion forward from geoStart to the requested kya —
// a fixed timeline keyed to the seed alone, so the same eruption
// happens at the same moment no matter where you scrub. Climate (sea
// level, mean temp delta) at the given kya is then applied per cell
// to derive whether the cell shows up as land, sea, glacier, or a
// fresh lava field. As kya scrubs from 205 toward 0, the ice retreats
// smoothly, Agraria submerges, volcanoes blow and their flows weather
// — all consequences of the two timelines, not hardcoded snapshots.
//
// Each stage below is a named transform over the World; ordering
// matters (later stages read the RegionIDs earlier stages wrote) and
// every stage must be deterministic — the snapshot tests pin the
// combined output.
func Generate(seed int64, kya int) World {
	return GenerateWithFates(seed, kya, nil)
}

// GenerateWithFates is Generate carrying the fate chain — the sealed
// records of the ages already simulated (fate.go). With a nil chain
// it is byte-identical to the pure equilibrium world the snapshot
// tests pin; with fates it folds the remembered ages in after the
// equilibrium seats stand and before lairs, roads, and the polity —
// so the old halls get roads, pressure, allegiance, and realms like
// anything else, and the tells of fallen houses scar the map.
func GenerateWithFates(seed int64, kya int, fates []Fate) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock, volcanoes, vsites, vsched := generateBedrock(rng, seed, kya)

	w := World{
		Seed:         seed,
		Kya:          kya,
		Era:          EraForKya(kya),
		LatTop:       DefaultLatTop,
		LatBottom:    DefaultLatBottom,
		Orbital:      OrbitalAt(kya),
		Climate:      ClimateAt(kya),
		Volcanoes:    volcanoes,
		volcanoSites: vsites,
		volcanoSched: vsched,
	}

	w.classifyRegions(bedrock)
	w.stampVolcanoes()

	// Rivers grow head-to-mouth as climate warms. Threshold is uniform
	// (it just identifies the river network topology); the maximum
	// length each river extends from its headwater scales with glacial
	// index — at the glacial peak, length=0 (no rivers; water locked in
	// ice). Lakes are a side-product of the same flow pass.
	var lakes []LakeCell
	var accum [][]int
	w.RiverInfo, w.Rivers, lakes, accum = flowRivers(bedrock,
		riverThreshold(),
		riverMaxLenFor(w.Climate.GlacialIndex))
	w.setDrainage(accum)
	w.nameRivers()
	w.computeDrainage(bedrock)

	w.applyLakes(lakes)
	w.refineBiomes()

	w.placeSeats()
	w.placeReaches()
	w.placeOutholds()
	w.foldFates(fates)
	w.placeLairs()
	w.applyDragonPressure()

	w.findPasses()
	w.buildRoads()
	w.markMarshes(lakes)
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
					Rock:      b.Rock,
					RockAge:   b.RockAgo,
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
//  4. Fresh lava — a flow younger than lavaFreshKa is raw black rock
//     no matter what zone it buried. It weathers back into the zone's
//     identity as it ages (the lithology remembers; the surface heals).
//  5. Otherwise the zone's exposed-land identity.
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
			// A land zone below sea level is a drowned valley — a
			// channel the glacial rivers cut at a low stand, flooded
			// when the warm sea came back.
			return RegionDrowned
		}
	}

	if b.Rock == RockLava && b.RockAgo <= lavaFreshKa {
		return RegionLava
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

// stampVolcanoes marks each born vent's summit cell. Runs right after
// classifyRegions and before any feature placement, so passes and
// lairs (which scan for plain mountain cells) skip the vents on their
// own.
func (w *World) stampVolcanoes() {
	if len(w.Volcanoes) == 0 {
		return
	}
	at := make(map[[2]int64]bool, len(w.Volcanoes))
	for _, v := range w.Volcanoes {
		at[[2]int64{v.X, v.Y}] = true
	}
	for i := range w.Regions {
		if at[[2]int64{w.Regions[i].X, w.Regions[i].Y}] {
			w.Regions[i].RegionID = RegionVolcano
		}
	}
}

// setDrainage stamps the flow-accumulation grid onto the region cells.
// Drainage is derived from the evolved bedrock, so it shifts with the
// geological history — a lava dam or a moraine belt reroutes it. The
// hydrology lens reads it.
//
// It also finishes the lithology: any land cell carrying real
// upstream flow is a working floodplain, and a working floodplain's
// surface is always young river silt — alluvium reworked flood by
// flood, no matter what the ice or the volcanoes left there before.
// Fresh lava resists (a river takes more than a season to cut a new
// flow); it weathers first.
func (w *World) setDrainage(accum [][]int) {
	for i := range w.Regions {
		rc := &w.Regions[i]
		rc.Drainage = int64(accum[rc.Y][rc.X])
		if rc.Drainage >= alluviumMinAccum && rc.Elevation > w.Climate.SeaLevelDelta &&
			!(rc.Rock == RockLava && rc.RockAge <= lavaFreshKa) {
			rc.Rock = RockAlluvium
			rc.RockAge = 0
		}
	}
}
