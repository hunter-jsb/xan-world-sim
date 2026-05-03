package world

import (
	"container/heap"
	"fmt"
	"sort"
)

// River carries the identity and a display name for a single river,
// independent of its actual cells. Stored in the rivers table.
//
// Drainage = number of rivers (transitively) that feed into this one,
// including itself. A leaf headwater stub has drainage 1; the trunk
// of a regional river system has the highest drainage. Lets consumers
// pick out the "Mississippi" of the cradle.
type River struct {
	ID       int64
	Name     string
	Drainage int64
}

// riverThreshold is the flow accumulation a cell needs to be in the
// river network *at all*. Uniform across climate — it just identifies
// which cells participate when a river is fully developed. What
// changes with climate is how far each river extends downstream from
// its headwater (riverMaxLenFor below).
//
// Tuning: higher value = fewer minor drainages, less scatter, more
// visible "main rivers." Lower = denser network, more contiguous
// chains but more minor stubs. Scales with map area: at 80×30 the
// land area is ~80% larger than the original 60×22 layout, so the
// threshold scales up to keep the network density similar.
const riverThreshold = 50

// riverMaxLenFor controls how many cells each river extends downstream
// from its headwater, as a function of glacial index. Rivers always
// start at headwaters; as the world warms, each river extends further
// downstream — head-to-mouth growth, matching deglaciation reality.
//
// Linear with no cap — gI=1.0 gives 0 (locked in ice), gI=0 gives
// max length. Smooth all the way through the cycle so panning kya
// shows steady extension instead of a jump-in at some threshold.
//
// Numbers (scaled for 80×30 grid; the constant 110 ≈ map diagonal):
//   gI = 1.00 → 0
//   gI = 0.85 → ~16  (rivers as small upstream segments)
//   gI = 0.50 → 55   (mid-Melt — reaching well into the cradle)
//   gI = 0.00 → 110  (full extent; major drainages reach the sea)
func riverMaxLenFor(gI float64) int {
	if gI >= 1.0 {
		return 0
	}
	return int(110.0 * (1.0 - gI))
}

// flowRivers runs a D8 flow-direction + flow-accumulation pass on the
// bedrock heightmap and returns the procgen-derived rivers. Each
// headwater becomes a river segment that traces downstream until it
// hits the sea, falls off the map, or merges with another river.
//
// Step 1: flow direction — each land cell points at its single lowest
// neighbor (steepest descent).
// Step 2: accumulation — each cell starts with 1 unit of rain; we sort
// cells by elevation descending and push their accumulation downhill.
// Step 3: trace — cells with accumulation >= threshold are river cells;
// each headwater becomes its own river_id, terminating where it
// reaches a previously-traced cell or leaves land.
// LakeCell is a single cell of a lake — a basin floor in the bedrock
// where pit-fill detected that flow would have to route uphill.
type LakeCell struct {
	X, Y int64
}

func flowRivers(bedrock [][]BedrockCell, threshold int, maxLen int) ([]River, []RiverCell, []LakeCell) {
	// Copy bedrock elevations into a fillable working field. We'll
	// raise pits in this copy without modifying bedrock — bedrock
	// stays the source of truth for visualization and sea checks.
	elev := make([][]float64, Height)
	for y := 0; y < Height; y++ {
		elev[y] = make([]float64, Width)
		for x := 0; x < Width; x++ {
			elev[y][x] = bedrock[y][x].Elevation
		}
	}
	fillPits(elev, bedrock)

	flowDir := computeFlowDirections(elev)
	accum := computeAccumulation(elev, bedrock, flowDir)
	rivers, riverCells := traceRivers(bedrock, flowDir, accum, threshold, maxLen)

	// Lake detection is a SYSTEM, not a hand-tuned filter:
	//
	//   geological — a cell is a basin candidate if pit-fill papered
	//                over a real concavity, i.e., its filled-flow
	//                target is higher in bedrock than itself. That's
	//                a direct algorithmic signature of a basin.
	//   scale — at our grid (each cell ≈ 50km on a Mediterranean-
	//           scale map), a single-cell candidate is sub-resolution
	//           noise. Real lakes have to span multiple cells to
	//           register. Cluster filter: a candidate becomes a lake
	//           only when it has at least one candidate neighbor.
	//   physics — frozen vs liquid is decided downstream in classify
	//             via Temperature() > 0. We don't filter here by
	//             climate; we just identify the geological feature.
	candidates := make(map[[2]int]bool)
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			z := bedrock[y][x].Zone
			if z != BZCradle && z != BZFoothill {
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
			if bedrock[ny][nx].Elevation <= 0 {
				continue
			}
			if bedrock[ny][nx].Elevation > bedrock[y][x].Elevation {
				candidates[[2]int{x, y}] = true
			}
		}
	}
	// Connected-component filter: walk candidates with BFS to find
	// each connected cluster, keep only clusters of meaningful size.
	// Real lakes span multiple cells; isolated 1-2 cell candidates
	// are noise at our grid resolution. The threshold of 3 is the
	// smallest cluster size that represents a real geographic feature
	// (~150km² at our cell size — comparable to real-world named
	// lakes), and is grounded in the resolution of the grid, not a
	// tunable knob.
	const minLakeClusterCells = 3
	lakeSet := make(map[[2]int]bool)
	visited := make(map[[2]int]bool)
	for c := range candidates {
		if visited[c] {
			continue
		}
		var component [][2]int
		queue := [][2]int{c}
		visited[c] = true
		for len(queue) > 0 {
			head := queue[0]
			queue = queue[1:]
			component = append(component, head)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					n := [2]int{head[0] + dx, head[1] + dy}
					if !candidates[n] || visited[n] {
						continue
					}
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		if len(component) >= minLakeClusterCells {
			for _, cell := range component {
				lakeSet[cell] = true
			}
		}
	}

	// Don't filter river cells just because they're also lake cells.
	// In real hydrology a river flows *through* a lake; we render the
	// river layer over the region layer, so a cell that's both will
	// show as a river — which is geologically correct.
	var lakes []LakeCell
	for cell := range lakeSet {
		lakes = append(lakes, LakeCell{X: int64(cell[0]), Y: int64(cell[1])})
	}
	return rivers, riverCells, lakes
}

// fillPits raises depressions in the heightmap so every land cell has
// a monotonically-decreasing path to the boundary (sea or map edge).
// Uses the priority-flood algorithm (Barnes et al. 2014): start with
// all boundary cells in a min-priority-queue; repeatedly pop the
// lowest, and for each unvisited neighbor, set its elevation to be
// at least (current + epsilon) before pushing it. After this pass,
// every cell sits above the lowest path to the boundary, so flow
// can always proceed downhill.
//
// We modify `elev` (a copy) and don't touch bedrock — bedrock keeps
// its original "real" cliffs and pits for visualization, while the
// flow algorithm runs on a hydrologically-clean version.
func fillPits(elev [][]float64, bedrock [][]BedrockCell) {
	const epsilon = 0.001
	visited := make([][]bool, Height)
	for y := range visited {
		visited[y] = make([]bool, Width)
	}

	var queue priorityQueue
	heap.Init(&queue)

	// Seed the queue with sea cells and all map-edge cells. These are
	// the boundary — water reaching them has somewhere to go.
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			isEdge := x == 0 || x == Width-1 || y == 0 || y == Height-1
			isSea := bedrock[y][x].Elevation <= 0
			if isEdge || isSea {
				heap.Push(&queue, pqItem{x: x, y: y, elev: elev[y][x]})
				visited[y][x] = true
			}
		}
	}

	dirs := [8]struct{ dx, dy int }{
		{-1, -1}, {0, -1}, {1, -1},
		{-1, 0}, {1, 0},
		{-1, 1}, {0, 1}, {1, 1},
	}

	for queue.Len() > 0 {
		top := heap.Pop(&queue).(pqItem)
		for _, d := range dirs {
			nx, ny := top.x+d.dx, top.y+d.dy
			if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
				continue
			}
			if visited[ny][nx] {
				continue
			}
			visited[ny][nx] = true
			// Don't raise sea cells — they're real sinks.
			if bedrock[ny][nx].Elevation > 0 && elev[ny][nx] <= top.elev {
				elev[ny][nx] = top.elev + epsilon
			}
			heap.Push(&queue, pqItem{x: nx, y: ny, elev: elev[ny][nx]})
		}
	}
}

type pqItem struct {
	x, y int
	elev float64
}

type priorityQueue []pqItem

func (h priorityQueue) Len() int            { return len(h) }
func (h priorityQueue) Less(i, j int) bool  { return h[i].elev < h[j].elev }
func (h priorityQueue) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *priorityQueue) Push(x interface{}) { *h = append(*h, x.(pqItem)) }
func (h *priorityQueue) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type flowVec struct{ dx, dy int }

func computeFlowDirections(elev [][]float64) [][]flowVec {
	out := make([][]flowVec, Height)
	for y := 0; y < Height; y++ {
		out[y] = make([]flowVec, Width)
		for x := 0; x < Width; x++ {
			self := elev[y][x]
			var best flowVec
			bestDrop := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					drop := self - elev[ny][nx]
					if drop > bestDrop {
						bestDrop = drop
						best = flowVec{dx, dy}
					}
				}
			}
			out[y][x] = best
		}
	}
	return out
}

func computeAccumulation(elev [][]float64, bedrock [][]BedrockCell, flowDir [][]flowVec) [][]int {
	accum := make([][]int, Height)
	type lc struct {
		x, y int
		elev float64
	}
	var cells []lc
	for y := 0; y < Height; y++ {
		accum[y] = make([]int, Width)
		for x := 0; x < Width; x++ {
			// Only land cells (per bedrock) generate rainfall.
			if bedrock[y][x].Elevation > 0 {
				accum[y][x] = 1
				cells = append(cells, lc{x, y, elev[y][x]})
			}
		}
	}
	sort.Slice(cells, func(i, j int) bool {
		return cells[i].elev > cells[j].elev
	})
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

func traceRivers(bedrock [][]BedrockCell, flowDir [][]flowVec, accum [][]int, threshold int, maxLen int) ([]River, []RiverCell) {
	// Only paint cells as rivers in zones where rivers visually
	// belong — cradle, foothill. Plateau and mountain accumulate
	// flow too, but their drainage in our 2D model often runs off the
	// north/south edges (no real-world equivalent of erosion-cut
	// gorges through the mountain ridge). Hiding rivers in those
	// zones gives the same shape as the hand-laid rivers — emerging
	// at the mountain-base foothills, crossing the cradle, exiting
	// at sea — without the plateau-top artifacts.
	isRiverZone := func(z BedrockZone) bool {
		return z == BZCradle || z == BZFoothill
	}
	isRiver := make(map[[2]int]bool)
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if accum[y][x] < threshold {
				continue
			}
			if !isRiverZone(bedrock[y][x].Zone) {
				continue
			}
			isRiver[[2]int{x, y}] = true
		}
	}
	// (Previously we filtered out cells whose flow went uphill in
	// bedrock — pit-fill artifacts. That broke chains visibly:
	// head→A→B→C with B uphill became "head→A" + "C", a gap at B.
	// Better contiguity to keep B in the chain and accept the rare
	// briefly-uphill segment; in real terrain those are small lakes
	// the river flows through.)

	// Headwaters: river cells with no upstream river cell flowing into them.
	incoming := make(map[[2]int]int)
	for cell := range isRiver {
		d := flowDir[cell[1]][cell[0]]
		if d.dx == 0 && d.dy == 0 {
			continue
		}
		target := [2]int{cell[0] + d.dx, cell[1] + d.dy}
		if isRiver[target] {
			incoming[target]++
		}
	}
	var headwaters [][2]int
	for cell := range isRiver {
		if incoming[cell] == 0 {
			headwaters = append(headwaters, cell)
		}
	}
	// Order headwaters deterministically (north -> south, west -> east)
	// so river ids are stable across runs of the same seed.
	sort.Slice(headwaters, func(i, j int) bool {
		if headwaters[i][1] != headwaters[j][1] {
			return headwaters[i][1] < headwaters[j][1]
		}
		return headwaters[i][0] < headwaters[j][0]
	})

	var rivers []River
	var cells []RiverCell
	visited := make(map[[2]int]bool)
	nextID := int64(1)

	for _, head := range headwaters {
		x, y := head[0], head[1]
		riverID := nextID
		var ord int64 = 1
		startedThisRiver := false
		for {
			if int(ord) > maxLen {
				break // climate-controlled river length cap
			}
			cell := [2]int{x, y}
			if visited[cell] {
				break // merged with another river
			}
			if !isRiver[cell] {
				break
			}
			visited[cell] = true
			cells = append(cells, RiverCell{
				RiverID: riverID, X: int64(x), Y: int64(y), Ord: ord,
			})
			ord++
			startedThisRiver = true
			d := flowDir[y][x]
			if d.dx == 0 && d.dy == 0 {
				break
			}
			x, y = x+d.dx, y+d.dy
			if x < 0 || x >= Width || y < 0 || y >= Height {
				break
			}
			if bedrock[y][x].Elevation <= 0 {
				break // hit sea
			}
		}
		if startedThisRiver {
			rivers = append(rivers, River{
				ID:   riverID,
				Name: fmt.Sprintf("River %d", riverID),
			})
			nextID++
		}
	}

	return rivers, cells
}
