package world

import "math/rand"

// Generate produces a deterministic world from the given seed and era.
// Bedrock geography (mountain row shape, plateau extent) is shared
// across eras; what changes is sea level, glacier extent, river
// presence, and which shelves are exposed vs drowned.
func Generate(seed int64, era Era) World {
	switch era {
	case EraOldWorld:
		return generateOldWorld(seed)
	default:
		return generateNow(seed)
	}
}

// generateNow is the post-Melt present: full Eastern Sea, Brine at
// present level, Agraria drowned, rivers flowing.
//
// RNG is layered on the hand-laid step functions:
//   - mountain row: random-walk jitter ±2 around the base
//   - foothill thickness: per-column ±1 around the base
//   - east coast: random-walk jitter ±2 around x=52
//
// Rivers are still hand-laid paths (jittering them risks crossing
// the jittered mountain — defer until rivers become a real flow sim).
func generateNow(seed int64) World {
	rng := rand.New(rand.NewSource(seed))

	mountainRow := genMountainRow(rng)
	foothillThick := genFoothillThickness(rng)
	coastX := genCoastX(rng)

	w := World{
		Seed: seed, Era: EraNow,
		LatTop: DefaultLatTop, LatBottom: DefaultLatBottom,
		Orbital: OrbitalForEra(EraNow),
		Climate: ClimateForEra(EraNow),
	}

	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			rid := classifyNow(x, y, mountainRow, foothillThick, coastX)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID: rid, X: int64(x), Y: int64(y),
				})
			}
		}
	}

	w.Rivers = staticRivers()
	return w
}

func classifyNow(x, y int, mountainRow, foothillThick, coastX []int) int64 {
	if x <= 1 {
		return RegionBrine
	}
	if x >= coastX[y] {
		return RegionEastSea
	}
	mr := mountainRow[x]
	if y < mr {
		return RegionPlateau
	}
	if y == mr {
		if x <= 11 {
			return RegionCliff
		}
		return RegionMountain
	}
	if isDoab(x, y) {
		return RegionDoab
	}
	if y > mr && y <= mr+foothillThick[x] {
		return RegionFoothill
	}
	return RegionCradle
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

func genMountainRow(rng *rand.Rand) []int {
	out := make([]int, Width)
	jitter := 0
	// walk from east to west so successive bands stay correlated
	for x := Width - 1; x >= 0; x-- {
		base := baseMountainRow(x)
		if base < 0 {
			out[x] = -1
			continue
		}
		jitter += rng.Intn(3) - 1 // -1, 0, +1
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
		ft := base + rng.Intn(3) - 1 // -1, 0, +1
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

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

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
	// Main river: (25,18) east to (51,18)
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
