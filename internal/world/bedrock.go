package world

import (
	"math"
	"math/rand"
	"sort"
)

// BedrockZone identifies the geological structure a cell belongs to.
// The zone layout is era-independent — the rift's architecture doesn't
// move on our timescales — but the rock itself lives: elevations and
// lithology evolve through the geological history in geology.go
// (uplift, volcanism, ice, isostasy, erosion), and the cell's *surface
// appearance* (sea / glacier / exposed land of various kinds) is then
// computed from zone + evolved elevation + the current climate state.
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

// Map allocation along x — all positions scale proportionally with
// Width and Height. Anchors are the 80×30 reference layout (well-tuned
// at that resolution); other sizes scale linearly. Widening the map
// just bumps the constants in world.go and everything below follows.
//
//	[0, brineEndX)              : Brine deep — always-submerged west
//	brineEndX                   : Agraria coast (deeper shelf)
//	brineEndX + 1               : Agraria upland
//	brineEndX + 2               : Agraria upland tapered
//	[landStartX, mountainEndX)  : Mountain row land (plateau / mountain
//	                              / foothill / cradle / doab)
//	[mountainEndX, eastSeaStart): Cradle extending east
//	[eastSeaStart, Width)       : Eastern Sea
const (
	// brineEndX = first land-strip column (right after the 4-col
	// brine band of the reference 80-wide layout). Scales as Width/20.
	brineEndX     = Width * 4 / 80
	agrariaCoastX = brineEndX
	// 3 agraria-strip columns (coast + upland + tapered) sit between
	// brine and land; landStartX is one past those.
	landStartX = brineEndX + 3
	// cliffEastX: at the SW end of the rift, the mountain row reads as
	// cliffs rather than mountains. This is the cutoff. ~26% of Width.
	cliffEastX = Width * 21 / 80
	// mountainEndX: mountain row stops here, leaving cradle space
	// between the eastern foothills and the Eastern Sea.
	mountainEndX = Width * 69 / 80
	// coastCenterX: Eastern Sea coastline jitter center.
	coastCenterX = Width * 70 / 80

	// Y-axis bounds for the Agraria strip cells (within the 80×30
	// reference: shelf 2..26, upland 2..22, tapered 5..19).
	shelfTopY      = Height * 2 / 30
	shelfBottomY   = Height * 26 / 30
	uplandTopY     = Height * 2 / 30
	uplandBottomY  = Height * 22 / 30
	taperedTopY    = Height * 5 / 30
	taperedBottomY = Height * 19 / 30

	// Mountain-row Y interpolation. The Rift slopes NE→SW: at the
	// far east the mountain band sits high (low Y), at the far west
	// it drops south to meet the Brine cliffs (high Y).
	mountainSouthY = Height * 19 / 30 // y at x=landStartX
	mountainNorthY = Height * 4 / 30  // y at x=mountainEndX-1
)

// BedrockCell is the geology at one (x,y) position at the generated
// moment — the output of the history integration in geology.go.
type BedrockCell struct {
	Zone      BedrockZone
	Elevation float64 // meters relative to present-day sea level (0)
	Rock      int64   // topmost lithology (Rock* constants in geology.go)
	RockAgo   int64   // ka before the generated moment the surface was laid
}

// roughElevationForZone is the initial height for erosion initialization.
// Erosion modifies these into physically-grounded terrain; this is a
// starting point, not the final value.
func roughElevationForZone(z BedrockZone) float64 {
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
		return 150
	case BZBrineDeep:
		return -800
	case BZAgrariaShelf:
		return -80
	case BZAgrariaUpland:
		return -40
	case BZEastBasin:
		return -150
	}
	return 0
}

// isLandZone returns true for zones above sea level that participate in erosion.
// Sea/shelf/basin zones are held fixed as boundary conditions.
func isLandZone(z BedrockZone) bool {
	switch z {
	case BZBrineDeep, BZAgrariaShelf, BZAgrariaUpland, BZEastBasin:
		return false
	}
	return true
}

// flowAccumulationFromElev counts upstream land cells for each cell.
// Mirrors computeAccumulation in rivers.go but uses elevation directly
// (before BedrockCells are finalized) to identify land cells.
func flowAccumulationFromElev(elev [][]float64, flowDir [][]flowVec) [][]int {
	accum := make([][]int, Height)
	type lc struct {
		x, y int
		e    float64
	}
	var cells []lc
	for y := 0; y < Height; y++ {
		accum[y] = make([]int, Width)
		for x := 0; x < Width; x++ {
			if elev[y][x] > 0 {
				accum[y][x] = 1
				cells = append(cells, lc{x, y, elev[y][x]})
			}
		}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].e > cells[j].e })
	for _, c := range cells {
		d := flowDir[c.y][c.x]
		if d.dx == 0 && d.dy == 0 {
			continue
		}
		nx, ny := c.x+d.dx, c.y+d.dy
		if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
			continue
		}
		accum[ny][nx] += accum[c.y][c.x]
	}
	return accum
}

// erodeStreamPower applies one explicit step of stream-power incision:
//
//	Δz = -K × A^m × S
//
// where A = upstream cell count, S = elevation drop to downhill neighbor.
// Sea cells act as 0m base level so rivers grade to sea level, not seafloor.
func erodeStreamPower(elev [][]float64, zones [][]BedrockZone, flowDir [][]flowVec, accum [][]int) {
	next := make([][]float64, Height)
	for y := range elev {
		next[y] = make([]float64, Width)
		copy(next[y], elev[y])
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if !isLandZone(zones[y][x]) {
				continue
			}
			d := flowDir[y][x]
			if d.dx == 0 && d.dy == 0 {
				continue
			}
			nx, ny := x+d.dx, y+d.dy
			if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
				continue
			}
			var downhill float64
			if isLandZone(zones[ny][nx]) {
				downhill = elev[ny][nx]
			}
			// Sea cells provide 0m base level — rivers carve to sea level.
			slope := elev[y][x] - downhill
			if slope <= 0 {
				continue
			}
			next[y][x] -= erosionK * math.Pow(float64(accum[y][x]), erosionM) * slope
		}
	}
	for y := range elev {
		copy(elev[y], next[y])
	}
}

// diffuseHillslope applies one step of hillslope diffusion, rounding sharp
// inter-cell steps into smoother gradients. Only land cells participate.
func diffuseHillslope(elev [][]float64, zones [][]BedrockZone) {
	next := make([][]float64, Height)
	for y := range elev {
		next[y] = make([]float64, Width)
		copy(next[y], elev[y])
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if !isLandZone(zones[y][x]) {
				continue
			}
			sum, count := 0.0, 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					if !isLandZone(zones[ny][nx]) {
						continue
					}
					sum += elev[ny][nx]
					count++
				}
			}
			if count == 0 {
				continue
			}
			next[y][x] = elev[y][x] + diffusionD*(sum/float64(count)-elev[y][x])
		}
	}
	for y := range elev {
		copy(elev[y], next[y])
	}
}

// Erosion model parameters for bedrock generation.
// Stream-power incision + hillslope diffusion run on the initial
// noise-seeded heights to produce physically-grounded terrain:
// trunk valleys carved by high-drainage flows, stable mountain peaks
// at drainage divides, smooth hillslope gradients in between.
const (
	erosionSteps = 15   // iterations to quasi-steady-state
	erosionK     = 2e-3 // stream-power erodibility; larger = faster valley carving
	erosionM     = 0.5  // drainage-area exponent (standard SPM value)
	diffusionD   = 0.02 // hillslope diffusivity; smooths inter-cell noise
)

// generateBedrock lives in geology.go now — the frame built here
// (zones, noise, spin-up helpers below) is the state of the world at
// geoStart, and the history integration carries it to the requested
// moment.

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
	// West water/shelf strip: brine band + 3-col shelf.
	// At kya=0 the shelf is submerged → entire strip reads as Brine;
	// at glacial peak the shelf emerges and the strip is half water /
	// half land. The shoreline literally lives on this strip.
	if x < brineEndX {
		return BZBrineDeep
	}
	if x == agrariaCoastX {
		if y >= shelfTopY && y <= shelfBottomY {
			return BZAgrariaShelf
		}
		return BZBrineDeep
	}
	if x == brineEndX+1 {
		if y >= uplandTopY && y <= uplandBottomY {
			return BZAgrariaUpland
		}
		return BZBrineDeep
	}
	if x == brineEndX+2 {
		// Tapered upland — narrower in y than the upland col,
		// suggests the shelf thinning out as it approaches the
		// plateau cliff base.
		if y >= taperedTopY && y <= taperedBottomY {
			return BZAgrariaUpland
		}
		return BZBrineDeep
	}

	// Eastern Sea strip (uses jittered coastX)
	if x >= coastX[y] {
		return BZEastBasin
	}

	// Inland strip
	mr := mountainRow[x]
	if mr >= 0 && y < mr {
		return BZPlateau
	}
	if mr >= 0 && y == mr {
		if x <= cliffEastX {
			return BZCliff
		}
		return BZMountain
	}
	if mr >= 0 && y > mr && y <= mr+foothillThick[x] {
		return BZFoothill
	}
	return BZCradle
}

// baseMountainRow returns the y-row of the mountain band at column x
// (or -1 if there is no mountain at this column). The Rift slopes
// NE→SW: at the eastern end the mountains sit high (low Y), at the
// far west they drop south (high Y) to meet the cliff coast.
//
// Computed as a linear interpolation between mountainSouthY (at
// x=landStartX) and mountainNorthY (at x=mountainEndX-1), so the
// band scales smoothly with Width and Height.
func baseMountainRow(x int) int {
	if x < landStartX || x >= mountainEndX {
		return -1
	}
	span := mountainEndX - 1 - landStartX
	if span <= 0 {
		return mountainSouthY
	}
	// pct: 0 at far west (south), 1 at far east (north)
	num := (x - landStartX) * (mountainSouthY - mountainNorthY)
	// Round-half-up to keep adjacent x in step.
	return mountainSouthY - (num+span/2)/span
}

// baseFoothillThickness returns the foothill band width at column x.
// Foothills broaden NE → SW... wait, opposite: they broaden NE (per
// lore: "NE end — asymptotic foothill blend... wide belt of rolling
// foothills"). The SW cliff section has 0 thickness. The east end has
// the maximum (3). Linearly interpolated across [cliffEastX, mountainEndX).
func baseFoothillThickness(x int) int {
	if x < cliffEastX || x >= mountainEndX {
		// SW cliffs and beyond-east cells: no foothill band here.
		return 0
	}
	span := mountainEndX - cliffEastX
	if span <= 0 {
		return 0
	}
	// 4 bands across the span (thickness 0,1,2,3).
	b := (x - cliffEastX) * 4 / span
	if b > 3 {
		b = 3
	}
	return b
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
		// Bounds (-3/+5 around center) are absolute cell counts, not
		// proportional to Width — the per-row jitter only swings ±2,
		// so a small fixed window is enough for any map size.
		out[y] = clamp(coastCenterX+jitter, coastCenterX-3, coastCenterX+5)
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
