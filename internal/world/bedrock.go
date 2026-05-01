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
	BZBrineDeep      // SW basin floor — too deep to expose or freeze
	BZAgrariaShelf   // NW continental shelf, lower (coast) — drowned now, exposes later as sea drops
	BZAgrariaUpland  // NW continental shelf, higher (upland) — drowned now but emerges first
	BZEastBasin      // basin east of the Rift — Eastern Sea now, ice sheet at glacial peak
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
		return -80 // coast: lower shelf, exposed when ΔSL < ~-80
	case BZAgrariaUpland:
		return -40 // upland: higher shelf, exposes first as sea drops
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
//
// Agraria layout (NW):
//   coast (x=0..1, y=4..17): AgrariaShelf — outer/west edge, deeper
//   upland (x=2..agrariaMaxX(y), y=4..14, north-of-mountain):
//                              AgrariaUpland — inner shelf with a
//                              tapered east boundary that bulges
//                              toward the cliff line
// The cliff line (mountain row in the SW) cuts through Agraria's
// range, so the cliff visually interrupts the shelf — geologically
// correct, because the cliffs are the western face of the plateau
// dropping down to the shelf.
func bedrockZone(x, y int, mountainRow, foothillThick, coastX []int) BedrockZone {
	// NW Agraria zone — shifted south, widened, tapered
	if y >= 4 && y <= 17 {
		if x <= 1 {
			return BZAgrariaShelf
		}
		if y <= 14 && x <= agrariaMaxX(y) {
			mr := mountainRow[x]
			if mr < 0 || y < mr {
				return BZAgrariaUpland
			}
		}
	}
	// West-most strip outside Agraria — deep Brine basin
	if x <= 1 {
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

// agrariaMaxX defines the east boundary of the Agraria upland — bulges
// in the middle latitudes where the shelf was widest, narrows back
// north and south.
func agrariaMaxX(y int) int {
	switch {
	case y >= 4 && y <= 6:
		return 4
	case y >= 7 && y <= 10:
		return 5
	case y >= 11 && y <= 14:
		return 7
	}
	return -1
}
