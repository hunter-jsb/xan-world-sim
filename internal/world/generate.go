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
// Rivers remain hand-laid for now and exist only when GlacialIndex
// is low (post-Melt-ish climate). The glacial-peak world has no
// rivers because the meltwater hasn't been released yet.
func Generate(seed int64, kya int) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock := generateBedrock(rng)

	climate := ClimateAt(kya)

	w := World{
		Seed:      seed,
		Kya:       kya,
		Era:       EraForKya(kya),
		LatTop:    DefaultLatTop,
		LatBottom: DefaultLatBottom,
		Orbital:   OrbitalAt(kya),
		Climate:   climate,
	}

	for y := 0; y < Height; y++ {
		lat := Latitude(y, w.LatTop, w.LatBottom)
		for x := 0; x < Width; x++ {
			b := bedrock[y][x]
			rid := classify(b, lat, climate)
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

	// Rivers are a post-Melt feature: meltwater has to be released
	// before the drainage network exists. Threshold of 0.3 ~= the
	// climate has retreated far enough from the glacial peak.
	if climate.GlacialIndex < 0.3 {
		w.Rivers = staticRivers()
	}
	return w
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

// ----- bedrock-procgen helpers (used by generateBedrock) -----

func genMountainRow(rng *rand.Rand) []int {
	out := make([]int, Width)
	jitter := 0
	for x := Width - 1; x >= 0; x-- {
		base := baseMountainRow(x)
		if base < 0 {
			out[x] = -1
			continue
		}
		jitter += rng.Intn(3) - 1
		jitter = clamp(jitter, -2, 2)
		mr := base + jitter
		mr = clamp(mr, 1, Height-3)
		out[x] = mr
	}
	return out
}

func genFoothillThickness(rng *rand.Rand) []int {
	out := make([]int, Width)
	for x := 0; x < Width; x++ {
		base := baseFoothillThickness(x)
		if base == 0 {
			out[x] = 0
			continue
		}
		ft := base + rng.Intn(3) - 1
		out[x] = clamp(ft, 0, 5)
	}
	return out
}

func genCoastX(rng *rand.Rand) []int {
	out := make([]int, Height)
	jitter := 0
	for y := 0; y < Height; y++ {
		jitter += rng.Intn(3) - 1
		jitter = clamp(jitter, -2, 2)
		out[y] = clamp(52+jitter, 50, 56)
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ----- rivers -----

func staticRivers() []RiverCell {
	type p struct{ x, y int }
	type r struct {
		id   int64
		path []p
	}
	// All x coords shifted east by 4 from the original layout (land
	// shifted to make room for visible Brine on the west).
	rivers := []r{
		{id: 1, path: []p{{21, 11}, {21, 12}, {21, 13}, {22, 14}, {23, 14}}},
		{id: 2, path: []p{{26, 10}, {26, 11}, {26, 12}, {25, 13}, {24, 14}, {23, 14}}},
		{id: 3, path: []p{{24, 15}, {25, 16}, {26, 17}, {27, 18}, {28, 18}}},
		{id: 4, path: []p{{32, 21}, {31, 20}, {30, 19}, {29, 18}, {28, 18}}},
		{id: 5, path: nil},
	}
	for x := 29; x <= 51; x++ {
		rivers[4].path = append(rivers[4].path, p{x, 18})
	}
	out := []RiverCell{}
	for _, riv := range rivers {
		for i, pt := range riv.path {
			out = append(out, RiverCell{
				RiverID: riv.id, X: int64(pt.x), Y: int64(pt.y), Ord: int64(i + 1),
			})
		}
	}
	return out
}
