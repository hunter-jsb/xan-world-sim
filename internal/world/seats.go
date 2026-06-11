package world

import "sort"

// placeSeats places the three core lord-seat tiers, each with a
// distinct geographic signature drawn from the lore typology in
// `region.md`:
//
//	Tributary — midpoint of a river of length ≥ 5. The salmon-lord
//	            hall on a navigable stretch. Scale-gated: shorter
//	            rivers can't sustain a lord at our cell size.
//	Headwater — head (Ord=1) of a river of length ≥ 10. Sacred
//	            sources at continental rivers; twice the Tributary
//	            scale because Headwater holds are bigger seats
//	            (closest to dwarves, contested by religious orders).
//	March     — foothill/cradle directly adjacent to a connected
//	            mountain massif of ≥3 cells. Geographically the
//	            "wall" — defense against the mountain wilds is the
//	            seat's reason to exist. One per massif, at the
//	            highest perimeter cell (most defensible).
//
// All three are climate-coupled through the layers below them (rivers
// vanish at LGM → no Tributary or Headwater; mountains stay → Marches
// persist through ice ages, which matches the lore: March lineages are
// the *oldest*, since the mountain is forever).
func (w *World) placeSeats() {
	seatSet := make(map[[2]int64]int64)
	if len(w.Rivers) > 0 {
		groups := make(map[int64][]RiverCell)
		for _, r := range w.Rivers {
			groups[r.RiverID] = append(groups[r.RiverID], r)
		}
		for _, group := range groups {
			sort.Slice(group, func(i, j int) bool { return group[i].Ord < group[j].Ord })
			if len(group) >= 5 {
				mid := group[len(group)/2]
				seatSet[[2]int64{mid.X, mid.Y}] = RegionSeat
			}
			if len(group) >= 10 {
				head := group[0]
				if _, taken := seatSet[[2]int64{head.X, head.Y}]; !taken {
					seatSet[[2]int64{head.X, head.Y}] = RegionHeadwater
				}
			}
		}
	}

	// March detection: flood mountain cells into massifs, then for each
	// massif of meaningful size pick the highest-elevation
	// foothill/cradle cell touching it. Don't overwrite existing seats
	// (Tributary or Headwater) — if the mountain massif's natural March
	// cell is already a river-tier seat, the role doubles up and we
	// just keep the river-tier label. This is fine: the typology in
	// lore explicitly bleeds (a Tributary on a wall-adjacent stretch
	// *is* a March in spirit).
	const minMassifCells = 3
	g := gridOf(w.Regions)
	isWallish := func(p [2]int) bool {
		switch g.regionAt(p) {
		case RegionFoothill, RegionCradle, RegionForest, RegionTundra, RegionMarsh:
			return true
		}
		return false
	}
	var seeds [][2]int
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			seeds = append(seeds, [2]int{x, y})
		}
	}
	isMountain := func(p [2]int) bool { return g.regionAt(p) == RegionMountain }
	for _, massif := range components(seeds, isMountain) {
		if len(massif) < minMassifCells {
			continue
		}
		best := [2]int{-1, -1}
		bestElev := -1e9
		for _, m := range massif {
			for _, d := range dirs8 {
				n := [2]int{m[0] + d[0], m[1] + d[1]}
				if !isWallish(n) {
					continue
				}
				if e := g.elevAt(n); e > bestElev {
					bestElev = e
					best = n
				}
			}
		}
		if best[0] < 0 {
			continue
		}
		key := [2]int64{int64(best[0]), int64(best[1])}
		if _, taken := seatSet[key]; !taken {
			seatSet[key] = RegionMarch
		}
	}

	if len(seatSet) == 0 {
		return
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		if id, ok := seatSet[[2]int64{rc.X, rc.Y}]; ok {
			rc.RegionID = id
		}
	}
	// Generate names for each seat. Seed mixes world seed with seat
	// coords, so the same hall on the same world always carries the
	// same name. Sorting by (y, x) before generating gives a stable
	// emission order — important for snapshot determinism.
	seatKeys := make([][2]int64, 0, len(seatSet))
	for k := range seatSet {
		seatKeys = append(seatKeys, k)
	}
	sortYX(seatKeys)
	for _, k := range seatKeys {
		w.Seats = append(w.Seats, NamedSeat{
			X:    k[0],
			Y:    k[1],
			Tier: seatSet[k],
			Name: generateName(nameSeedForCell(w.Seed, k[0], k[1])),
		})
	}
	// Filter river cells that landed on a seat — the directional
	// river glyph would paint over the seat marker. River presence
	// is implicit (the seat sits on it).
	filtered := w.Rivers[:0]
	for _, r := range w.Rivers {
		if _, ok := seatSet[[2]int64{r.X, r.Y}]; ok {
			continue
		}
		filtered = append(filtered, r)
	}
	w.Rivers = filtered
}

// placeReaches places the frontier-explorer seat tier. "A seat at the
// far edge of crown reach... so remote it is essentially autonomous in
// practice. Crown couriers arrive late or never."
//
// Heartland is defined as the centroid of the Tributary seats — that's
// where the salmon-lord halls cluster, which is the crown's actual
// logistical reach. A Reach is among the K seat-eligible cells
// maximally far from this centroid, with greedy spatial dedup so
// different Reaches sit in different cardinal directions.
//
// K scales with the number of Tributaries (one Reach per ~3
// Tributaries, min 1 max 4) — a world with no heartland (no
// Tributaries, e.g., LGM) gets no Reaches; a world with a sprawling
// crown gets several frontier holds at its periphery.
func (w *World) placeReaches() {
	var sumX, sumY float64
	var nTrib int
	for _, s := range w.Seats {
		if s.Tier == RegionSeat {
			sumX += float64(s.X)
			sumY += float64(s.Y)
			nTrib++
		}
	}
	if nTrib == 0 {
		return
	}
	cx := sumX / float64(nTrib)
	cy := sumY / float64(nTrib)
	seatAt := make(map[[2]int64]bool, len(w.Seats))
	for _, s := range w.Seats {
		seatAt[[2]int64{s.X, s.Y}] = true
	}
	type scored struct {
		x, y int64
		d    float64
	}
	var cands []scored
	for i := range w.Regions {
		rc := &w.Regions[i]
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionTundra, RegionFoothill:
		default:
			continue
		}
		if seatAt[[2]int64{rc.X, rc.Y}] {
			continue
		}
		dx := float64(rc.X) - cx
		dy := float64(rc.Y) - cy
		cands = append(cands, scored{rc.X, rc.Y, dx*dx + dy*dy})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].d != cands[j].d {
			return cands[i].d > cands[j].d
		}
		if cands[i].y != cands[j].y {
			return cands[i].y < cands[j].y
		}
		return cands[i].x < cands[j].x
	})
	// K from Tributary count: one Reach per 3 Tributaries, at least 1
	// and at most 4. Grid-justified: minSep = 6 cells (~300km at our
	// scale), the scale at which two distant frontier holds clearly
	// belong to different "reaches" of crown rather than being
	// neighbors.
	k := min(max(nTrib/3, 1), 4)
	const minSepSq = 6 * 6
	var picks []scored
	for _, c := range cands {
		tooClose := false
		for _, p := range picks {
			ddx := c.x - p.x
			ddy := c.y - p.y
			if ddx*ddx+ddy*ddy < minSepSq {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		picks = append(picks, c)
		if len(picks) >= k {
			break
		}
	}
	reachSet := make(map[[2]int64]bool, len(picks))
	for _, p := range picks {
		reachSet[[2]int64{p.x, p.y}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		if reachSet[[2]int64{rc.X, rc.Y}] {
			rc.RegionID = RegionReach
		}
	}
	sort.Slice(picks, func(i, j int) bool {
		if picks[i].y != picks[j].y {
			return picks[i].y < picks[j].y
		}
		return picks[i].x < picks[j].x
	})
	for _, p := range picks {
		w.Seats = append(w.Seats, NamedSeat{
			X:    p.x,
			Y:    p.y,
			Tier: RegionReach,
			Name: generateName(nameSeedForCell(w.Seed, p.x, p.y)),
		})
	}
}

// placeOutholds places the catch-all seat tier from the lore.
// "Off-river, off-grid, no formal frontier role." Detected as the
// strict local maxima of distance from any civilization (rivers +
// named seats). This is the geographic signature of remoteness, scale-
// grounded by the BFS over the grid: a cell is an Outhold candidate
// only if it's *farther from civ than every neighbor*, so they emerge
// naturally spaced apart, never clumped.
//
// Minimum distance of 3 cells = ~150km buffer at our cell size, the
// scale at which a "remote holding" is meaningfully separated from the
// nearest road / river / hall. Smaller wouldn't be remote; larger
// wouldn't fit our grid.
func (w *World) placeOutholds() {
	const minOutholdDist = 3
	const inf = 1 << 30
	dist := make([][]int, Height)
	for y := range dist {
		dist[y] = make([]int, Width)
		for x := range dist[y] {
			dist[y][x] = inf
		}
	}
	type qPt struct{ x, y, d int }
	var bfs []qPt
	mark := func(x, y int) {
		if !inBounds(x, y) {
			return
		}
		if dist[y][x] != 0 {
			dist[y][x] = 0
			bfs = append(bfs, qPt{x, y, 0})
		}
	}
	for _, r := range w.Rivers {
		mark(int(r.X), int(r.Y))
	}
	for _, s := range w.Seats {
		mark(int(s.X), int(s.Y))
	}
	for i := 0; i < len(bfs); i++ {
		c := bfs[i]
		for _, d := range dirs8 {
			nx, ny := c.x+d[0], c.y+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			nd := c.d + 1
			if nd < dist[ny][nx] {
				dist[ny][nx] = nd
				bfs = append(bfs, qPt{nx, ny, nd})
			}
		}
	}
	// Find strict local maxima among livable cells.
	var picks [][2]int
	for i := range w.Regions {
		rc := &w.Regions[i]
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionTundra:
		default:
			continue
		}
		d := dist[int(rc.Y)][int(rc.X)]
		if d == inf || d < minOutholdDist {
			continue
		}
		// Local-max with E/S tiebreaker: a cell wins a tie against its
		// eastern and southern neighbors, but loses ties to north/west.
		// This selects exactly one cell per connected plateau of
		// equal-distance, no clustering.
		isMax := true
		for _, dd := range dirs8 {
			nx, ny := int(rc.X)+dd[0], int(rc.Y)+dd[1]
			if !inBounds(nx, ny) {
				continue
			}
			nd := dist[ny][nx]
			if nd > d {
				isMax = false
			} else if nd == d {
				// tie: lose only to N/W neighbors
				if dd[1] < 0 || (dd[1] == 0 && dd[0] < 0) {
					isMax = false
				}
			}
			if !isMax {
				break
			}
		}
		if isMax {
			picks = append(picks, [2]int{int(rc.X), int(rc.Y)})
		}
	}
	// Apply: flip cells, append to Seats. Sort for stable hash order.
	sortYX(picks)
	outholdSet := make(map[[2]int64]bool, len(picks))
	for _, p := range picks {
		outholdSet[[2]int64{int64(p[0]), int64(p[1])}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		if outholdSet[[2]int64{rc.X, rc.Y}] {
			rc.RegionID = RegionOuthold
		}
	}
	for _, p := range picks {
		w.Seats = append(w.Seats, NamedSeat{
			X:    int64(p[0]),
			Y:    int64(p[1]),
			Tier: RegionOuthold,
			Name: generateName(nameSeedForCell(w.Seed, int64(p[0]), int64(p[1]))),
		})
	}
}

// applyDragonPressure computes per-seat exposure to dragon raids.
// Lore: "Northern kingdoms — those nestled up against the Mountain
// Barrier — live under constant dragon pressure... Risk falls off with
// distance from the mountains." Computed as
//
//	pressure = max(0, raidRadius - chebyshev_distance_to_nearest_den)
//
// at our cell size, raidRadius=12 cells ≈ 600km — the scale at which a
// dragon's territory tapers off into safe heartland.
func (w *World) applyDragonPressure() {
	if len(w.Dens) == 0 || len(w.Seats) == 0 {
		return
	}
	const raidRadius = 12
	for i := range w.Seats {
		s := &w.Seats[i]
		minD := raidRadius
		for _, d := range w.Dens {
			dx := int(s.X - d.X)
			if dx < 0 {
				dx = -dx
			}
			dy := int(s.Y - d.Y)
			if dy < 0 {
				dy = -dy
			}
			minD = min(minD, max(dx, dy))
		}
		if p := raidRadius - minD; p > 0 {
			s.Pressure = float64(p)
		}
	}
}
