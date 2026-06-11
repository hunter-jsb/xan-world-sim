package world

import "sort"

// dirs8 enumerates the 8 neighbor offsets in row-major scan order
// (NW, N, NE, W, E, SW, S, SE). Every neighbor walk in the package
// uses this order — BFS discovery order and tie-breaking depend on
// it, so it must stay consistent for snapshot determinism.
var dirs8 = [8][2]int{
	{-1, -1}, {0, -1}, {1, -1},
	{-1, 0}, {1, 0},
	{-1, 1}, {0, 1}, {1, 1},
}

func inBounds(x, y int) bool {
	return x >= 0 && x < Width && y >= 0 && y < Height
}

// cellGrid is an O(1) coordinate view over a World's region cells.
// Stages that mutate RegionIDs must rebuild it (gridOf) before the
// next read — it's a snapshot, not a live view.
type cellGrid struct {
	region map[[2]int]int64
	elev   map[[2]int]float64
}

func gridOf(regions []RegionCell) *cellGrid {
	g := &cellGrid{
		region: make(map[[2]int]int64, len(regions)),
		elev:   make(map[[2]int]float64, len(regions)),
	}
	for _, rc := range regions {
		p := [2]int{int(rc.X), int(rc.Y)}
		g.region[p] = rc.RegionID
		g.elev[p] = rc.Elevation
	}
	return g
}

// regionAt returns the RegionID at p, or 0 for cells with no region
// (off-map or never classified) — same semantics as the zero value of
// the ad-hoc maps this replaces.
func (g *cellGrid) regionAt(p [2]int) int64 { return g.region[p] }

func (g *cellGrid) elevAt(p [2]int) float64 { return g.elev[p] }

// sortYX orders points by (y, x) — the canonical emission order for
// snapshot determinism.
func sortYX[T int | int64](pts [][2]T) {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][1] != pts[j][1] {
			return pts[i][1] < pts[j][1]
		}
		return pts[i][0] < pts[j][0]
	})
}

// components returns the 8-connected components of the cell set
// defined by member, seeded in the order given. Cells within each
// component appear in BFS discovery order (seed first, then dirs8
// expansion) — callers that resolve ties by first-found rely on this.
// Seeds must be supplied in a deterministic order.
func components(seeds [][2]int, member func(p [2]int) bool) [][][2]int {
	visited := make(map[[2]int]bool)
	var out [][][2]int
	for _, s := range seeds {
		if visited[s] || !member(s) {
			continue
		}
		var comp [][2]int
		queue := [][2]int{s}
		visited[s] = true
		for len(queue) > 0 {
			head := queue[0]
			queue = queue[1:]
			comp = append(comp, head)
			for _, d := range dirs8 {
				n := [2]int{head[0] + d[0], head[1] + d[1]}
				if member(n) && !visited[n] {
					visited[n] = true
					queue = append(queue, n)
				}
			}
		}
		out = append(out, comp)
	}
	return out
}
