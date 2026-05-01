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
	BZBrineDeep     // SW basin floor — too deep to expose or freeze
	BZAgrariaShelf  // shelf coast — drowned now, exposes later as sea drops
	BZAgrariaUpland // shelf upland — drowned now but emerges first
	BZEastBasin     // basin east of the Rift — Eastern Sea now, ice sheet at glacial peak
)

// Map allocation along x:
//   x =  0..2  : Brine deep (always submerged — gives the shoreline real
//                visible water to retreat from / advance into)
//   x =  3     : Agraria coast (deeper shelf, exposes later)
//   x =  4..5  : Agraria upland (higher shelf, exposes first, tapers
//                in y to suggest a natural wedge against the plateau)
//   x =  6..51 : Land (plateau / mountain / foothill / cradle / doab)
//   x = 52..59 : Eastern Sea (unchanged)
const (
	landStartX  = 6
	agrariaCoastX = 3
)

// BedrockCell is the era-independent geology at one (x,y) position.
type BedrockCell struct {
	Zone      BedrockZone
	Elevation float64 // meters relative to present-day sea level (0)
}

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

func bedrockZone(x, y int, mountainRow, foothillThick, coastX []int) BedrockZone {
	// West water/shelf strip: 3 cols of pure Brine + 3 cols of shelf.
	// At kya=0 the shelf is submerged → entire strip reads as Brine;
	// at glacial peak the shelf emerges and the strip is half water /
	// half land. The shoreline literally lives on this strip.
	if x <= 2 {
		return BZBrineDeep
	}
	if x == agrariaCoastX {
		if y >= 2 && y <= 18 {
			return BZAgrariaShelf
		}
		return BZBrineDeep
	}
	if x == 4 {
		if y >= 2 && y <= 14 {
			return BZAgrariaUpland
		}
		return BZBrineDeep
	}
	if x == 5 {
		// Tapered upland — narrower than x=4, suggests the shelf
		// thinning out as it approaches the plateau cliff base.
		if y >= 4 && y <= 12 {
			return BZAgrariaUpland
		}
		return BZBrineDeep
	}

	// Eastern Sea strip (uses jittered coastX)
	if x >= coastX[y] {
		return BZEastBasin
	}

	// Inland (x=6..51)
	mr := mountainRow[x]
	if mr >= 0 && y < mr {
		return BZPlateau
	}
	if mr >= 0 && y == mr {
		if x <= 15 {
			return BZCliff
		}
		return BZMountain
	}
	if isDoab(x, y) {
		return BZDoab
	}
	if mr >= 0 && y > mr && y <= mr+foothillThick[x] {
		return BZFoothill
	}
	return BZCradle
}

// baseMountainRow returns the y-row of the mountain band at column x
// (or -1 if there is no mountain at this column). All ranges shifted
// east by 4 from the original layout — the easternmost band (mr=2)
// would have collided with the Eastern Sea so it's dropped.
func baseMountainRow(x int) int {
	switch {
	case x >= 48 && x <= 51:
		return 3
	case x >= 44 && x <= 47:
		return 4
	case x >= 40 && x <= 43:
		return 5
	case x >= 36 && x <= 39:
		return 6
	case x >= 32 && x <= 35:
		return 7
	case x >= 28 && x <= 31:
		return 8
	case x >= 24 && x <= 27:
		return 9
	case x >= 20 && x <= 23:
		return 10
	case x >= 16 && x <= 19:
		return 11
	case x >= 12 && x <= 15:
		return 12
	case x >= 8 && x <= 11:
		return 13
	case x >= 6 && x <= 7:
		return 14
	}
	return -1
}

func baseFoothillThickness(x int) int {
	switch {
	case x >= 6 && x <= 15:
		return 0
	case x >= 16 && x <= 27:
		return 1
	case x >= 28 && x <= 39:
		return 2
	case x >= 40 && x <= 51:
		return 3
	}
	return 0
}

// isDoab — shifted east by 4 from old coords (was x=18..21).
func isDoab(x, y int) bool {
	if x >= 22 && x <= 25 && (y == 11 || y == 12) {
		return true
	}
	if x >= 22 && x <= 24 && y == 13 {
		return true
	}
	return false
}
