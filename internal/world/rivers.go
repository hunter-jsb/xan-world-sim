package world

import (
	"container/heap"
	"fmt"
	"sort"
)

// River carries the identity and a display name for a single river,
// independent of its actual cells. Stored in the rivers table.
type River struct {
	ID   int64
	Name string
}

// riverThreshold is the flow accumulation a cell needs to be marked
// as a river. Tuned for our ~60x22 grid: high enough that only the
// major drainages light up, not every minor rivulet. If you change
// map dimensions or generate-rain rules, retune.
const riverThreshold = 50

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
func flowRivers(bedrock [][]BedrockCell) ([]River, []RiverCell) {
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
	return traceRivers(bedrock, flowDir, accum)
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

func traceRivers(bedrock [][]BedrockCell, flowDir [][]flowVec, accum [][]int) ([]River, []RiverCell) {
	// Only paint cells as rivers in zones where rivers visually
	// belong — cradle, foothill, doab. Plateau and mountain accumulate
	// flow too, but their drainage in our 2D model often runs off the
	// north/south edges (no real-world equivalent of erosion-cut
	// gorges through the mountain ridge). Hiding rivers in those
	// zones gives the same shape as the hand-laid rivers — emerging
	// at the mountain-base foothills, crossing the cradle, exiting
	// at sea — without the plateau-top artifacts.
	isRiverZone := func(z BedrockZone) bool {
		return z == BZCradle || z == BZFoothill || z == BZDoab
	}
	isRiver := make(map[[2]int]bool)
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if accum[y][x] >= riverThreshold && isRiverZone(bedrock[y][x].Zone) {
				isRiver[[2]int{x, y}] = true
			}
		}
	}

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
