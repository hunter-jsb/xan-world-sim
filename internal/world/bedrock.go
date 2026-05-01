package world

import "math/rand"

// BedrockZone identifies the geological structure a cell belongs to.
// Bedrock is era-independent — geology is stable over the timescales
// we care about. The cell's *surface appearance* (sea / glacier /
// exposed land of various kinds) is computed from the bedrock zone,
// the bedrock elevation, and the current climate state.
type BedrockZone uint8

const (
	BZUnknown BedrockZone = iota
	BZPlateau
	BZMountain
	BZCliff
	BZFoothill
	BZCradle
	BZDoab
	BZBrineDeep    // SW basin floor — too deep to expose or freeze
	BZAgrariaShelf // NW continental shelf — drowned now, exposed at glacial peaks
	BZEastBasin    // basin east of the Rift — Eastern Sea now, ice sheet at glacial peak
)

// BedrockCell is the era-independent geology at one (x,y) position.
type BedrockCell struct {
	Zone      BedrockZone
	Elevation float64 // meters relative to present-day sea level (0)
}

// elevationForZone returns the canonical elevation for a bedrock zone.
// These are the numbers temperature() and sea-level checks consume.
// Currently fixed per zone (no per-cell jitter); jitter can be added
// later if we want elevation-noisy glaciation/coastlines.
func elevationForZone(z BedrockZone) float64 {
	switch z {
	case BZPlateau:
		return 1500
	case BZMountain:
		return 3000
	case BZCliff:
		return 2500
	case BZFoothill:
		return 500
	case BZDoab:
		return 2000
	case BZCradle:
		return 100
	case BZBrineDeep:
		return -800
	case BZAgrariaShelf:
		return -80 // just below present sea level — exposed when ΔSL < ~-80
	case BZEastBasin:
		return -150 // shallower basin — Eastern Sea floor
	}
	return 0
}

// generateBedrock produces the era-independent geology for the whole map
// from a seeded RNG. Reuses the same step-function-with-jitter approach
// established in earlier passes: mountain row + foothill thickness + east
// coast all walked with bounded random jitter for natural irregularity.
func generateBedrock(rng *rand.Rand) [][]BedrockCell {
	mountainRow := genMountainRow(rng)
	foothillThick := genFoothillThickness(rng)
	coastX := genCoastX(rng)

	out := make([][]BedrockCell, Height)
	for y := 0; y < Height; y++ {
		out[y] = make([]BedrockCell, Width)
		for x := 0; x < Width; x++ {
			z := bedrockZone(x, y, mountainRow, foothillThick, coastX)
			out[y][x] = BedrockCell{Zone: z, Elevation: elevationForZone(z)}
		}
	}
	return out
}

// bedrockZone classifies one cell's geology from the procgen-derived
// row/column metadata. Pure function once the metadata is fixed.
func bedrockZone(x, y int, mountainRow, foothillThick, coastX []int) BedrockZone {
	if x <= 1 {
		if y <= 7 {
			return BZAgrariaShelf
		}
		return BZBrineDeep
	}
	if x >= coastX[y] {
		return BZEastBasin
	}
	mr := mountainRow[x]
	if y < mr {
		return BZPlateau
	}
	if y == mr {
		if x <= 11 {
			return BZCliff
		}
		return BZMountain
	}
	if isDoab(x, y) {
		return BZDoab
	}
	if y > mr && y <= mr+foothillThick[x] {
		return BZFoothill
	}
	return BZCradle
}
