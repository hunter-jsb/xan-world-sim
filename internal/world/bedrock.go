package world

import (
	"math"
	"math/rand"
)

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

// Heightmap generation parameters. Tuned to give every land zone real
// internal variation (so rivers will have downhill gradients to follow
// in v2), while preserving the big drops at cliff/coastline boundaries.
const (
	smoothPasses    = 2     // box-filter passes; more = more diffuse
	smoothThreshold = 500.0 // don't average across drops bigger than this — preserves cliffs and coasts
	smoothWeight    = 0.3   // how strongly each pass blends self with neighborhood
)

func generateBedrock(rng *rand.Rand) [][]BedrockCell {
	mountainRow := genMountainRow(rng)
	foothillThick := genFoothillThickness(rng)
	coastX := genCoastX(rng)

	// Phase 1: compute the bedrock zone for every cell.
	zones := make([][]BedrockZone, Height)
	for y := 0; y < Height; y++ {
		zones[y] = make([]BedrockZone, Width)
		for x := 0; x < Width; x++ {
			zones[y][x] = bedrockZone(x, y, mountainRow, foothillThick, coastX)
		}
	}

	// Phase 2: zone-base + per-cell noise. Each zone has its own
	// noise amplitude — mountains are jagged (±500m), plateaus are
	// rolling (±200m), cradle is gentle (±50m), shelves and basin
	// floors get small noise (±15..100m). RNG order: y outer, x
	// inner, every cell consumes one Float64.
	elev := make([][]float64, Height)
	for y := 0; y < Height; y++ {
		elev[y] = make([]float64, Width)
		for x := 0; x < Width; x++ {
			base := elevationForZone(zones[y][x])
			amp := zoneAmplitude(zones[y][x])
			elev[y][x] = base + (rng.Float64()*2-1)*amp
		}
	}

	// Phase 3: smoothing passes. For each cell, blend its elevation
	// toward the average of its 8 neighbors — but only counting
	// neighbors whose elevation is within smoothThreshold (so we
	// preserve real terrain features: cliffs, mountain edges,
	// coastlines all stay sharp). Within-zone noise gets smoothed
	// into coherent gradients that water can flow down.
	for pass := 0; pass < smoothPasses; pass++ {
		next := make([][]float64, Height)
		for y := 0; y < Height; y++ {
			next[y] = make([]float64, Width)
			for x := 0; x < Width; x++ {
				self := elev[y][x]
				sum, count := self, 1.0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						nx, ny := x+dx, y+dy
						if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
							continue
						}
						n := elev[ny][nx]
						if math.Abs(n-self) > smoothThreshold {
							continue
						}
						sum += n
						count++
					}
				}
				avg := sum / count
				next[y][x] = self*(1-smoothWeight) + avg*smoothWeight
			}
		}
		elev = next
	}

	// Phase 4: pack into BedrockCells.
	out := make([][]BedrockCell, Height)
	for y := 0; y < Height; y++ {
		out[y] = make([]BedrockCell, Width)
		for x := 0; x < Width; x++ {
			out[y][x] = BedrockCell{Zone: zones[y][x], Elevation: elev[y][x]}
		}
	}
	return out
}

// zoneAmplitude is the maximum elevation deviation (in meters) added
// as noise to cells of this zone. Calibrated to feel realistic for
// the kind of terrain each zone represents.
func zoneAmplitude(z BedrockZone) float64 {
	switch z {
	case BZPlateau:
		return 200 // rolling tableland
	case BZMountain:
		return 500 // peaks and saddles
	case BZCliff:
		return 200 // jagged but constrained
	case BZFoothill:
		return 100 // hills, modest variation
	case BZDoab:
		return 200 // mountainous wedge
	case BZCradle:
		return 50 // mostly flat lowland
	case BZBrineDeep:
		return 100 // ocean floor variation
	case BZAgrariaShelf:
		return 15 // shelf surface — matches previous tuning
	case BZAgrariaUpland:
		return 15
	case BZEastBasin:
		return 50 // shallower basin floor
	}
	return 0
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
		if y >= 2 && y <= 19 {
			return BZAgrariaShelf
		}
		return BZBrineDeep
	}
	if x == 4 {
		if y >= 2 && y <= 16 {
			return BZAgrariaUpland
		}
		return BZBrineDeep
	}
	if x == 5 {
		// Tapered upland — narrower than x=4, suggests the shelf
		// thinning out as it approaches the plateau cliff base.
		if y >= 4 && y <= 14 {
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
