package world

import (
	"fmt"
	"sort"

	"github.com/hunterjsb/xan-world-sim/internal/pqueue"
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
// visible "main rivers." Lower = denser network. Scales with the
// number of land cells — at the 80×30 reference layout this is 50;
// for larger maps we want proportionally more accumulation upstream
// before a stream qualifies as a river.
const riverThresholdRef = 50
const riverThresholdRefArea = 80 * 30

func riverThreshold() int {
	t := riverThresholdRef * Width * Height / riverThresholdRefArea
	if t < 5 {
		return 5
	}
	return t
}

// riverMaxLenFor controls how many cells each river extends downstream
// from its headwater, as a function of glacial index. Rivers always
// start at headwaters; as the world warms, each river extends further
// downstream — head-to-mouth growth, matching deglaciation reality.
//
// Linear with no cap — gI=1.0 gives 0 (locked in ice), gI=0 gives
// max length. Smooth all the way through the cycle so panning kya
// shows steady extension instead of a jump-in at some threshold.
//
// Numbers scale to the map diagonal — at 80×30 that's ~85 cells, and
// we want full-warm rivers to potentially reach across the whole
// thing, so the cap is ≈ Width + Height (≈110 at 80×30).
func riverMaxLenFor(gI float64) int {
	if gI >= 1.0 {
		return 0
	}
	maxLen := float64(Width + Height)
	return int(maxLen * (1.0 - gI))
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
// LakeCell is a single submerged cell of a lake — a basin floor that
// sits below its basin's spill level. Surface is the water surface
// elevation (the spill level, shared by every cell of the same lake);
// Depth is how far this cell's bedrock sits below that surface.
type LakeCell struct {
	X, Y    int64
	Surface float64
	Depth   float64
}

func flowRivers(bedrock [][]BedrockCell, threshold int, maxLen int) ([]River, []RiverCell, []LakeCell, [][]int) {
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

	// Lake detection is a SYSTEM, grounded directly in the pit-fill
	// physics — basin-overflow modeling rather than a flow heuristic:
	//
	//   geological — fillPits raises every basin cell to the basin's
	//                spill level, which is exactly the water surface a
	//                lake reaches before it overflows. The raised
	//                amount (filled − bedrock) IS the water depth at
	//                that cell. depth > 0 means the cell is submerged.
	//   scale — open water is detected with depth hysteresis against
	//           the zone's noise amplitude. A lake needs an *anchor*
	//           deeper than amp/2 (cradle 25m, foothill 50m — a pit
	//           the ± amp noise can't produce as texture), and its
	//           *body* is the anchor's connected shelf deeper than
	//           amp/4 — attached to a deep reservoir, so it stays
	//           wet. Submergence shallower than amp/4 is the basin's
	//           seasonal fringe and reads as land (rendering it
	//           flooded the whole lowland with lake-webs). The basin
	//           and its spill surface are still established from the
	//           full submerged web (≥1m, which excludes pit-fill's
	//           epsilon grading). Bodies need ≥ 3 cells (~150km²) —
	//           the smallest feature the grid can honestly represent.
	//   physics — frozen vs liquid is decided downstream in applyLakes
	//             via Temperature() > 0. We don't filter here by
	//             climate; we just identify the geological feature.
	const submergedMin = 1.0
	const minLakeClusterCells = 3
	submerged := make(map[[2]int]bool)
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			z := bedrock[y][x].Zone
			if z != BZCradle && z != BZFoothill {
				continue
			}
			if elev[y][x]-bedrock[y][x].Elevation >= submergedMin {
				submerged[[2]int{x, y}] = true
			}
		}
	}
	var seeds [][2]int
	for c := range submerged {
		seeds = append(seeds, c)
	}
	sortYX(seeds)

	// One water body = one surface. Priority-flood assigns every cell
	// the spill level of *its own* depression, so two adjacent
	// submerged cells with different fill levels belong to different
	// basins — separate lakes, possibly terraced down a valley. Flood
	// with surface continuity: neighbors join only when their fill
	// levels match within surfTol. Within a single basin the fill is
	// uniform up to epsilon grading (0.001/cell, ≲0.5m across any
	// path); across a saddle into another basin it jumps by meters.
	const surfTol = 0.5
	var lakes []LakeCell
	visited := make(map[[2]int]bool)
	for _, s := range seeds {
		if visited[s] {
			continue
		}
		comp := [][2]int{s}
		visited[s] = true
		for i := 0; i < len(comp); i++ {
			head := comp[i]
			hf := elev[head[1]][head[0]]
			for _, d := range dirs8 {
				n := [2]int{head[0] + d[0], head[1] + d[1]}
				if !submerged[n] || visited[n] {
					continue
				}
				nf := elev[n[1]][n[0]]
				if nf-hf > surfTol || hf-nf > surfTol {
					continue // different basin (different water surface)
				}
				visited[n] = true
				comp = append(comp, n)
			}
		}
		// Surface = the basin's spill level (highest fill in the web).
		var surface float64
		for _, cell := range comp {
			if f := elev[cell[1]][cell[0]]; f > surface {
				surface = f
			}
		}
		// Hysteresis: collect the amp/4 shelf, cluster it spatially,
		// and keep only clusters that contain an amp/2 anchor and
		// span at least minLakeClusterCells.
		shelf := make(map[[2]int]bool)
		var shelfSeeds [][2]int
		isAnchor := make(map[[2]int]bool)
		for _, cell := range comp {
			d := surface - bedrock[cell[1]][cell[0]].Elevation
			amp := zoneAmplitude(bedrock[cell[1]][cell[0]].Zone)
			if d >= amp/4 {
				shelf[cell] = true
				shelfSeeds = append(shelfSeeds, cell)
			}
			if d >= amp/2 {
				isAnchor[cell] = true
			}
		}
		sortYX(shelfSeeds)
		for _, body := range components(shelfSeeds, func(p [2]int) bool { return shelf[p] }) {
			if len(body) < minLakeClusterCells {
				continue
			}
			anchored := false
			for _, cell := range body {
				if isAnchor[cell] {
					anchored = true
					break
				}
			}
			if !anchored {
				continue
			}
			for _, cell := range body {
				lakes = append(lakes, LakeCell{
					X:       int64(cell[0]),
					Y:       int64(cell[1]),
					Surface: surface,
					Depth:   surface - bedrock[cell[1]][cell[0]].Elevation,
				})
			}
		}
	}
	return rivers, riverCells, lakes, accum
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

	queue := pqueue.New(func(a, b pqItem) bool { return a.elev < b.elev })

	// Seed the queue with sea cells and all map-edge cells. These are
	// the boundary — water reaching them has somewhere to go.
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			isEdge := x == 0 || x == Width-1 || y == 0 || y == Height-1
			isSea := bedrock[y][x].Elevation <= 0
			if isEdge || isSea {
				queue.Push(pqItem{x: x, y: y, elev: elev[y][x]})
				visited[y][x] = true
			}
		}
	}

	for queue.Len() > 0 {
		top := queue.Pop()
		for _, d := range dirs8 {
			nx, ny := top.x+d[0], top.y+d[1]
			if !inBounds(nx, ny) {
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
			queue.Push(pqItem{x: nx, y: ny, elev: elev[ny][nx]})
		}
	}
}

type pqItem struct {
	x, y int
	elev float64
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

// nameRivers replaces placeholder "River N" labels with seeded phoneme
// names. Naming is anchored to each river's headwater coords + world
// seed, not to its ID — so the same river retains its name across kya
// even though its length scales with climate.
func (w *World) nameRivers() {
	if len(w.RiverInfo) == 0 {
		return
	}
	headOf := make(map[int64]RiverCell, len(w.RiverInfo))
	for _, rc := range w.Rivers {
		if rc.Ord == 1 {
			headOf[rc.RiverID] = rc
		}
	}
	for i := range w.RiverInfo {
		head, ok := headOf[w.RiverInfo[i].ID]
		if !ok {
			continue
		}
		w.RiverInfo[i].Name = generateName(
			nameSeedForCell(w.Seed, head.X, head.Y))
	}
}

// computeDrainage counts, for each river, how many other rivers
// (including itself) flow into it transitively. The merge target is
// detected from the river's tail cell: among 8-neighbors that sit on a
// *different* river, pick the one with lowest bedrock elevation
// (steepest descent). That neighbor's river is the merge target. If no
// such neighbor exists, the river reaches sea or boundary — it's a
// "trunk" candidate.
//
// Drainage propagation: each river contributes 1 to itself and to
// every ancestor in its merge chain. The river with maximum drainage
// is the cradle's "Mississippi" from the lore.
func (w *World) computeDrainage(bedrock [][]BedrockCell) {
	if len(w.RiverInfo) == 0 {
		return
	}
	groups := make(map[int64][]RiverCell, len(w.RiverInfo))
	for _, r := range w.Rivers {
		groups[r.RiverID] = append(groups[r.RiverID], r)
	}
	for id := range groups {
		sort.Slice(groups[id], func(i, j int) bool { return groups[id][i].Ord < groups[id][j].Ord })
	}
	riverAt := make(map[[2]int]int64, len(w.Rivers))
	for _, r := range w.Rivers {
		riverAt[[2]int{int(r.X), int(r.Y)}] = r.RiverID
	}
	mergeTarget := make(map[int64]int64, len(w.RiverInfo))
	for id, group := range groups {
		tail := group[len(group)-1]
		tx, ty := int(tail.X), int(tail.Y)
		// flowRivers stops a chain when it would walk into a cell
		// already claimed by another river — so the tail's flow
		// direction *must* lead into another river's cell. We don't
		// have flowDir here; we approximate by picking the 8-neighbor
		// on a different river with the lowest bedrock elevation
		// (steepest descent target). Don't compare against the tail's
		// elevation because pit-fill artifacts can leave the merge
		// target slightly higher in raw bedrock terms — what we know
		// for sure is the chain ended because *some* adjacent
		// different-river cell was the next flow step.
		var bestID int64 = -1
		bestElev := 1e18
		for _, d := range dirs8 {
			nx, ny := tx+d[0], ty+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			nID, ok := riverAt[[2]int{nx, ny}]
			if !ok || nID == id {
				continue
			}
			if nElev := bedrock[ny][nx].Elevation; nElev < bestElev {
				bestElev = nElev
				bestID = nID
			}
		}
		if bestID > 0 {
			mergeTarget[id] = bestID
		}
	}
	drainage := make(map[int64]int64, len(w.RiverInfo))
	// Each river contributes 1 to itself and 1 to each ancestor.
	// Visited set guards against pathological cycles in mergeTarget
	// (the elevation-min heuristic for merge detection isn't truly
	// guaranteed acyclic, even though flow direction is).
	for _, ri := range w.RiverInfo {
		cur := ri.ID
		drainage[cur]++
		visited := map[int64]bool{cur: true}
		for {
			next, ok := mergeTarget[cur]
			if !ok || visited[next] {
				break
			}
			drainage[next]++
			visited[next] = true
			cur = next
		}
	}
	for i := range w.RiverInfo {
		w.RiverInfo[i].Drainage = drainage[w.RiverInfo[i].ID]
	}
}
