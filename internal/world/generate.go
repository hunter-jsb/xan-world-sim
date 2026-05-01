package world

import "math/rand"

// Generate produces a deterministic world from the given seed and era.
//
// The pipeline is climate-driven: a single bedrock model (zones +
// elevations) is built once from the seed, then the era's ClimateState
// (sea level, mean temp delta) is applied per cell to derive whether
// that cell shows up as land, sea, or glacier. So glacier extent and
// coastlines *emerge* from the climate rather than being painted per
// era.
//
// Rivers remain hand-laid for now; they exist only at EraNow because
// they're a Melt-era feature (the glacial-peak world has no rivers).
func Generate(seed int64, era Era) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock := generateBedrock(rng)

	w := World{
		Seed: seed, Era: era,
		LatTop:    DefaultLatTop,
		LatBottom: DefaultLatBottom,
		Orbital:   OrbitalForEra(era),
		Climate:   ClimateForEra(era),
	}

	for y := 0; y < Height; y++ {
		lat := Latitude(y, w.LatTop, w.LatBottom)
		for x := 0; x < Width; x++ {
			rid := classify(bedrock[y][x], lat, w.Climate)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID: rid, X: int64(x), Y: int64(y),
				})
			}
		}
	}

	if era == EraNow {
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

	if b.Zone == BZAgrariaShelf && b.Elevation >= seaLevel {
		return RegionAgraria
	}

	if canGlaciate(b.Zone) {
		if Temperature(lat, b.Elevation, climate) < glacierThreshold {
			return RegionGlacier
		}
	}

	if b.Elevation < seaLevel {
		switch b.Zone {
		case BZBrineDeep, BZAgrariaShelf:
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

func baseMountainRow(x int) int {
	switch {
	case x >= 48 && x <= 51:
		return 2
	case x >= 44 && x <= 47:
		return 3
	case x >= 40 && x <= 43:
		return 4
	case x >= 36 && x <= 39:
		return 5
	case x >= 32 && x <= 35:
		return 6
	case x >= 28 && x <= 31:
		return 7
	case x >= 24 && x <= 27:
		return 8
	case x >= 20 && x <= 23:
		return 9
	case x >= 16 && x <= 19:
		return 10
	case x >= 12 && x <= 15:
		return 11
	case x >= 8 && x <= 11:
		return 12
	case x >= 4 && x <= 7:
		return 13
	case x >= 2 && x <= 3:
		return 14
	}
	return -1
}

func baseFoothillThickness(x int) int {
	switch {
	case x >= 2 && x <= 11:
		return 0
	case x >= 12 && x <= 23:
		return 1
	case x >= 24 && x <= 35:
		return 2
	case x >= 36 && x <= 51:
		return 3
	}
	return 0
}

func isDoab(x, y int) bool {
	if x >= 18 && x <= 21 && (y == 11 || y == 12) {
		return true
	}
	if x >= 18 && x <= 20 && y == 13 {
		return true
	}
	return false
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
	rivers := []r{
		{id: 1, path: []p{{17, 11}, {17, 12}, {17, 13}, {18, 14}, {19, 14}}},
		{id: 2, path: []p{{22, 10}, {22, 11}, {22, 12}, {21, 13}, {20, 14}, {19, 14}}},
		{id: 3, path: []p{{20, 15}, {21, 16}, {22, 17}, {23, 18}, {24, 18}}},
		{id: 4, path: []p{{28, 21}, {27, 20}, {26, 19}, {25, 18}, {24, 18}}},
		{id: 5, path: nil},
	}
	for x := 25; x <= 51; x++ {
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
