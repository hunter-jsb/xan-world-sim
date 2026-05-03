package world

import (
	"math/rand"
	"sort"
)

// Generate produces a deterministic world from the given seed and a
// moment in geological time (kya = kiloyears before present).
//
// The pipeline is climate-driven and time-driven: a single bedrock
// model (zones + elevations) is built once from the seed and is
// stable across all kya — geology doesn't move on these timescales.
// Climate (sea level, mean temp delta) at the given kya is then
// applied per cell to derive whether the cell shows up as land,
// sea, or glacier. As kya scrubs from 205 toward 0, the ice retreats
// smoothly and Agraria submerges — both as consequences of the
// climate cycle, not hardcoded snapshots.
//
// Rivers remain hand-laid for now and exist only when GlacialIndex
// is low (post-Melt-ish climate). The glacial-peak world has no
// rivers because the meltwater hasn't been released yet.
func Generate(seed int64, kya int) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock := generateBedrock(rng)

	climate := ClimateAt(kya)

	w := World{
		Seed:      seed,
		Kya:       kya,
		Era:       EraForKya(kya),
		LatTop:    DefaultLatTop,
		LatBottom: DefaultLatBottom,
		Orbital:   OrbitalAt(kya),
		Climate:   climate,
	}

	for y := 0; y < Height; y++ {
		lat := Latitude(y, w.LatTop, w.LatBottom)
		for x := 0; x < Width; x++ {
			b := bedrock[y][x]
			rid := classify(b, lat, climate)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID:  rid,
					X:         int64(x),
					Y:         int64(y),
					Elevation: b.Elevation,
				})
			}
		}
	}

	// Rivers grow head-to-mouth as climate warms. Threshold is uniform
	// (it just identifies the river network topology); the maximum
	// length each river extends from its headwater scales with
	// glacial index. At the glacial peak, length=0 (no rivers — water
	// locked in ice). As warming progresses, headwaters appear first,
	// then rivers extend downstream. By the time the cycle is fully
	// warm, rivers reach all the way from headwater to sea.
	//
	// Lakes are a side-product: cells where pit-fill identified a
	// basin floor (flow target is higher in bedrock terms). Convert
	// eligible cradle/foothill cells in-place so the renderer paints
	// them as lakes. Cells that are currently glaciated (e.g., at the
	// glacial peak when the cradle is under ice) keep their glacier
	// classification — lakes are buried until the ice retreats, which
	// is geologically correct.
	var lakes []LakeCell
	w.RiverInfo, w.Rivers, lakes = flowRivers(bedrock,
		riverThreshold,
		riverMaxLenFor(climate.GlacialIndex))

	// Replace placeholder "River N" labels with seeded phoneme names.
	// Naming is anchored to each river's headwater coords + world seed,
	// not to its ID — so the same river retains its name across kya
	// even though its length scales with climate.
	if len(w.RiverInfo) > 0 {
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
				nameSeedForCell(seed, head.X, head.Y))
		}
	}

	// Drainage — for each river, count how many other rivers (including
	// itself) flow into it transitively. The merge target is detected
	// from the river's tail cell: among 8-neighbors that sit on a
	// *different* river, pick the one with lowest bedrock elevation
	// (steepest descent). That neighbor's river is the merge target.
	// If no such neighbor exists, the river reaches sea or boundary —
	// it's a "trunk" candidate.
	//
	// Drainage propagation: each river contributes 1 to itself and to
	// every ancestor in its merge chain. The river with maximum
	// drainage is the cradle's "Mississippi" from the lore.
	if len(w.RiverInfo) > 0 {
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
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := tx+dx, ty+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					nID, ok := riverAt[[2]int{nx, ny}]
					if !ok || nID == id {
						continue
					}
					nElev := bedrock[ny][nx].Elevation
					if nElev < bestElev {
						bestElev = nElev
						bestID = nID
					}
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

	if len(lakes) > 0 {
		lakeSet := make(map[[2]int]bool, len(lakes))
		for _, l := range lakes {
			lakeSet[[2]int{int(l.X), int(l.Y)}] = true
		}
		// A geological basin renders as a *lake* only when the cell's
		// temperature is above freezing. Below freezing, the basin
		// holds ice — and our classifier already routes cold land to
		// RegionGlacier, so non-glaciated cradle/foothill in basins
		// is exactly the "liquid" case. Temperature is recomputed per
		// cell because lapse rate makes higher cells colder than
		// lower cells at the same latitude.
		for i := range w.Regions {
			rc := &w.Regions[i]
			if !lakeSet[[2]int{int(rc.X), int(rc.Y)}] {
				continue
			}
			if rc.RegionID != RegionCradle && rc.RegionID != RegionFoothill {
				continue
			}
			lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
			if Temperature(lat, rc.Elevation, climate) > 0 {
				rc.RegionID = RegionLake
			}
		}
	}

	// Biome refinement: split bare cradle cells by temperature into
	// forest (cool temperate) or tundra (cold but unfrozen). The two
	// gates are real ecological transitions:
	//   0°C  — water freezes year-round; below this trees can't sustain
	//          a closed canopy and we're in tundra territory.
	//   15°C — closed temperate forest gives way to warmer
	//          grassland/maquis above this. (Real-world MAT for the
	//          temperate-warm transition; matches our cradle's
	//          intended Mediterranean flavor at warmer values.)
	// Foothills keep their topographic identity (the `n` glyph
	// represents *hills*, not vegetation) so we don't biome-shift them.
	const (
		freezePoint     = 0.0
		warmCradleStart = 15.0
	)
	for i := range w.Regions {
		rc := &w.Regions[i]
		if rc.RegionID != RegionCradle {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		t := Temperature(lat, rc.Elevation, climate)
		switch {
		case t < freezePoint:
			rc.RegionID = RegionTundra
		case t < warmCradleStart:
			rc.RegionID = RegionForest
		}
	}

	// Lord seats — three tiers, each with a distinct geographic
	// signature drawn from the lore typology in `region.md`:
	//
	//   Tributary — midpoint of a river of length ≥ 5. The salmon-lord
	//               hall on a navigable stretch. Scale-gated: shorter
	//               rivers can't sustain a lord at our cell size.
	//   Headwater — head (Ord=1) of a river of length ≥ 10. Sacred
	//               sources at continental rivers; twice the Tributary
	//               scale because Headwater holds are bigger seats
	//               (closest to dwarves, contested by religious orders).
	//   March    — foothill/cradle directly adjacent to a connected
	//              mountain massif of ≥3 cells. Geographically the
	//              "wall" — defense against the mountain wilds is the
	//              seat's reason to exist. One per massif, at the
	//              highest perimeter cell (most defensible).
	//
	// All three are climate-coupled through the layers below them
	// (rivers vanish at LGM → no Tributary or Headwater; mountains
	// stay → Marches persist through ice ages, which matches the lore:
	// March lineages are the *oldest*, since the mountain is forever).
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
	// March detection: BFS-flood mountain cells into massifs, then for
	// each massif of meaningful size pick the highest-elevation
	// foothill/cradle cell touching it. Don't overwrite existing seats
	// (Tributary or Headwater) — if the mountain massif's natural
	// March cell is already a river-tier seat, the role doubles up
	// and we just keep the river-tier label. This is fine: the
	// typology in lore explicitly bleeds (a Tributary on a wall-
	// adjacent stretch *is* a March in spirit).
	{
		const minMassifCells = 3
		regionAt := make(map[[2]int]int64, len(w.Regions))
		elevAt := make(map[[2]int]float64, len(w.Regions))
		for _, rc := range w.Regions {
			regionAt[[2]int{int(rc.X), int(rc.Y)}] = rc.RegionID
			elevAt[[2]int{int(rc.X), int(rc.Y)}] = rc.Elevation
		}
		visited := make(map[[2]int]bool)
		isMountain := func(p [2]int) bool { return regionAt[p] == RegionMountain }
		isWallish := func(p [2]int) bool {
			id := regionAt[p]
			return id == RegionFoothill || id == RegionCradle ||
				id == RegionForest || id == RegionTundra ||
				id == RegionMarsh
		}
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				p := [2]int{x, y}
				if !isMountain(p) || visited[p] {
					continue
				}
				var massif [][2]int
				queue := [][2]int{p}
				visited[p] = true
				for len(queue) > 0 {
					head := queue[0]
					queue = queue[1:]
					massif = append(massif, head)
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if dx == 0 && dy == 0 {
								continue
							}
							n := [2]int{head[0] + dx, head[1] + dy}
							if isMountain(n) && !visited[n] {
								visited[n] = true
								queue = append(queue, n)
							}
						}
					}
				}
				if len(massif) < minMassifCells {
					continue
				}
				best := [2]int{-1, -1}
				bestElev := -1e9
				for _, m := range massif {
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if dx == 0 && dy == 0 {
								continue
							}
							n := [2]int{m[0] + dx, m[1] + dy}
							if !isWallish(n) {
								continue
							}
							if elevAt[n] > bestElev {
								bestElev = elevAt[n]
								best = n
							}
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
		}
	}
	if len(seatSet) > 0 {
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
		sort.Slice(seatKeys, func(i, j int) bool {
			if seatKeys[i][1] != seatKeys[j][1] {
				return seatKeys[i][1] < seatKeys[j][1]
			}
			return seatKeys[i][0] < seatKeys[j][0]
		})
		for _, k := range seatKeys {
			w.Seats = append(w.Seats, NamedSeat{
				X:    k[0],
				Y:    k[1],
				Tier: seatSet[k],
				Name: generateName(nameSeedForCell(seed, k[0], k[1])),
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

	// Doab: the wedge of land between two converging rivers (real
	// geographic term — Mesopotamia is the classic example; lore calls
	// our specific one "the jewel of the north"). Detection is purely
	// systemic — a cell qualifies if multiple distinct river IDs sit
	// within its local neighborhood, which geometrically picks out the
	// interior of the V/Y formed by converging chains.
	//
	// Radius is grid-justified: at our cell size (~50km), neighborhood
	// radius 2 = ~100km, which is the natural scale of "wedge between
	// two rivers" before they're far enough apart to belong to
	// independent drainages. Smaller would miss legitimate wedges;
	// larger would smear unrelated rivers together.
	//
	// Restricted to foothill/cradle land — sea, lakes, mountains,
	// glaciers, agraria etc. don't read as doab. Skip cells that are
	// already seats (the lore's "lord who controls the choke" sits
	// *in* the doab; the seat keeps its identity, not the doab tag).
	{
		const doabRadius = 2
		riverIDsAt := make(map[[2]int]int64, len(w.Rivers))
		for _, r := range w.Rivers {
			riverIDsAt[[2]int{int(r.X), int(r.Y)}] = r.RiverID
		}
		for i := range w.Regions {
			rc := &w.Regions[i]
			switch rc.RegionID {
			case RegionCradle, RegionFoothill, RegionForest, RegionTundra:
			default:
				continue
			}
			seen := make(map[int64]bool)
			for dy := -doabRadius; dy <= doabRadius; dy++ {
				for dx := -doabRadius; dx <= doabRadius; dx++ {
					n := [2]int{int(rc.X) + dx, int(rc.Y) + dy}
					if rid, ok := riverIDsAt[n]; ok {
						seen[rid] = true
					}
				}
			}
			if len(seen) >= 2 {
				rc.RegionID = RegionDoab
			}
		}
	}

	// Reach — the frontier-explorer seat tier. "A seat at the far edge
	// of crown reach... so remote it is essentially autonomous in
	// practice. Crown couriers arrive late or never."
	//
	// Heartland is defined as the centroid of the Tributary seats —
	// that's where the salmon-lord halls cluster, which is the crown's
	// actual logistical reach. A Reach is among the K seat-eligible
	// cells maximally far from this centroid, with greedy spatial
	// dedup so different Reaches sit in different cardinal directions.
	//
	// K scales with the number of Tributaries (one Reach per ~3
	// Tributaries, min 1 max 4) — a world with no heartland (no
	// Tributaries, e.g., LGM) gets no Reaches; a world with a sprawling
	// crown gets several frontier holds at its periphery.
	{
		var sumX, sumY float64
		var nTrib int
		for _, s := range w.Seats {
			if s.Tier == RegionSeat {
				sumX += float64(s.X)
				sumY += float64(s.Y)
				nTrib++
			}
		}
		if nTrib > 0 {
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
			// K from Tributary count: one Reach per 3 Tributaries, at
			// least 1 and at most 4. Grid-justified: minSep = 6 cells
			// (~300km at our scale), the scale at which two distant
			// frontier holds clearly belong to different "reaches" of
			// crown rather than being neighbors.
			k := nTrib / 3
			if k < 1 {
				k = 1
			}
			if k > 4 {
				k = 4
			}
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
					Name: generateName(nameSeedForCell(seed, p.x, p.y)),
				})
			}
		}
	}

	// Outhold — the catch-all seat tier from the lore. "Off-river,
	// off-grid, no formal frontier role." Detected as the strict
	// local maxima of distance from any civilization (rivers + named
	// seats). This is the geographic signature of remoteness, scale-
	// grounded by the BFS over the grid: a cell is an Outhold candidate
	// only if it's *farther from civ than every neighbor*, so they
	// emerge naturally spaced apart, never clumped.
	//
	// Minimum distance of 3 cells = ~150km buffer at our cell size, the
	// scale at which a "remote holding" is meaningfully separated from
	// the nearest road / river / hall. Smaller wouldn't be remote;
	// larger wouldn't fit our grid.
	{
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
			if x < 0 || x >= Width || y < 0 || y >= Height {
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
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := c.x+dx, c.y+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					nd := c.d + 1
					if nd < dist[ny][nx] {
						dist[ny][nx] = nd
						bfs = append(bfs, qPt{nx, ny, nd})
					}
				}
			}
		}
		// Find strict local maxima among livable cells.
		type outhold struct{ x, y int }
		var picks []outhold
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
			// Local-max with E/S tiebreaker: a cell wins a tie against
			// its eastern and southern neighbors, but loses ties to
			// north/west. This selects exactly one cell per connected
			// plateau of equal-distance, no clustering.
			isMax := true
			for dy := -1; dy <= 1 && isMax; dy++ {
				for dx := -1; dx <= 1 && isMax; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := int(rc.X)+dx, int(rc.Y)+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					nd := dist[ny][nx]
					if nd > d {
						isMax = false
					} else if nd == d {
						// tie: lose only to N/W neighbors
						if dy < 0 || (dy == 0 && dx < 0) {
							isMax = false
						}
					}
				}
			}
			if isMax {
				picks = append(picks, outhold{int(rc.X), int(rc.Y)})
			}
		}
		// Apply: flip cells, append to Seats. Sort for stable hash order.
		sort.Slice(picks, func(i, j int) bool {
			if picks[i].y != picks[j].y {
				return picks[i].y < picks[j].y
			}
			return picks[i].x < picks[j].x
		})
		outholdSet := make(map[[2]int64]bool, len(picks))
		for _, p := range picks {
			outholdSet[[2]int64{int64(p.x), int64(p.y)}] = true
		}
		for i := range w.Regions {
			rc := &w.Regions[i]
			if outholdSet[[2]int64{rc.X, rc.Y}] {
				rc.RegionID = RegionOuthold
			}
		}
		for _, p := range picks {
			w.Seats = append(w.Seats, NamedSeat{
				X:    int64(p.x),
				Y:    int64(p.y),
				Tier: RegionOuthold,
				Name: generateName(nameSeedForCell(seed, int64(p.x), int64(p.y))),
			})
		}
	}

	// Mountain passes — saddles in the ridge that bridge the cradle
	// to the plateau. From the lore: "pre-Melt these were passable;
	// the Melt made them spectacular and brutal." Detection signals:
	//   1. The cell is itself a mountain (it sits *on* the ridge).
	//   2. Its elevation is ≤ all 8-neighbor mountain cells (locally
	//      lowest along the ridge axis — the saddle).
	//   3. It has at least one foothill/cradle/forest/tundra cell to
	//      its south — meaning the cradle side is reachable from
	//      this point. Without (3) the saddle dead-ends inside the
	//      mountain band and isn't a real "pass through."
	// E/S tiebreaker on equal-elevation neighbors so a flat ridge-top
	// doesn't yield clusters of passes.
	{
		regionAt := make(map[[2]int]int64, len(w.Regions))
		elevAt := make(map[[2]int]float64, len(w.Regions))
		for _, rc := range w.Regions {
			regionAt[[2]int{int(rc.X), int(rc.Y)}] = rc.RegionID
			elevAt[[2]int{int(rc.X), int(rc.Y)}] = rc.Elevation
		}
		isApproachKind := func(id int64) bool {
			return id == RegionFoothill || id == RegionCradle ||
				id == RegionForest || id == RegionTundra ||
				id == RegionMarsh
		}
		// 5x5 window for the local-min check: passes are "the lowest
		// cell in the ridge for ~250km around" at our cell size.
		// A 3x3 window over-counts because every short rise+dip in the
		// smoothed elevation registers; 5x5 only flags cells that
		// dominate a meaningful stretch of ridge.
		const passWindow = 2
		var picks [][2]int
		for i := range w.Regions {
			rc := &w.Regions[i]
			if rc.RegionID != RegionMountain {
				continue
			}
			cx, cy := int(rc.X), int(rc.Y)
			d := elevAt[[2]int{cx, cy}]
			isMin := true
			hasMtnNbr := false
			hasApproach := false
			for dy := -passWindow; dy <= passWindow && isMin; dy++ {
				for dx := -passWindow; dx <= passWindow && isMin; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					n := [2]int{cx + dx, cy + dy}
					nid, nok := regionAt[n]
					if !nok {
						continue
					}
					if nid == RegionMountain {
						hasMtnNbr = true
						nd := elevAt[n]
						if nd < d {
							isMin = false
						} else if nd == d {
							// E/S tiebreaker: lose ties to N/W
							if dy < 0 || (dy == 0 && dx < 0) {
								isMin = false
							}
						}
					}
					// "South approach" remains the immediate row below
					// (we want a foothill/cradle directly accessible
					// from the saddle, not several cells away).
					if dy == 1 && (dx >= -1 && dx <= 1) && isApproachKind(nid) {
						hasApproach = true
					}
				}
			}
			if isMin && hasMtnNbr && hasApproach {
				picks = append(picks, [2]int{cx, cy})
			}
		}
		sort.Slice(picks, func(i, j int) bool {
			if picks[i][1] != picks[j][1] {
				return picks[i][1] < picks[j][1]
			}
			return picks[i][0] < picks[j][0]
		})
		passSet := make(map[[2]int64]bool, len(picks))
		for _, p := range picks {
			passSet[[2]int64{int64(p[0]), int64(p[1])}] = true
		}
		for i := range w.Regions {
			rc := &w.Regions[i]
			if passSet[[2]int64{rc.X, rc.Y}] {
				rc.RegionID = RegionPass
			}
		}
		var nextID int64 = 1
		for _, p := range picks {
			w.Passes = append(w.Passes, PassInfo{
				ID:   nextID,
				Name: generateName(nameSeedForCell(seed, int64(p[0]), int64(p[1]))),
				X:    int64(p[0]),
				Y:    int64(p[1]),
			})
			nextID++
		}
	}

	// Marsh: vegetated lowland directly adjacent to a water body, where
	// temperature is above freezing. The "adjacency to water" criterion
	// is the wet-biome definition; the temperature gate is the same
	// freezing-point used for lakes — frozen wetlands aren't marshes.
	waterSet := make(map[[2]int]bool, len(w.Regions)+len(w.Rivers))
	for _, rc := range w.Regions {
		switch rc.RegionID {
		case RegionLake, RegionBrine, RegionEastSea:
			waterSet[[2]int{int(rc.X), int(rc.Y)}] = true
		}
	}
	for _, r := range w.Rivers {
		waterSet[[2]int{int(r.X), int(r.Y)}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionTundra:
		default:
			continue
		}
		// Check 8-neighbors for water adjacency.
		adjacent := false
		for dy := -1; dy <= 1 && !adjacent; dy++ {
			for dx := -1; dx <= 1 && !adjacent; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				if waterSet[[2]int{int(rc.X) + dx, int(rc.Y) + dy}] {
					adjacent = true
				}
			}
		}
		if !adjacent {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		if Temperature(lat, rc.Elevation, climate) > freezePoint {
			rc.RegionID = RegionMarsh
		}
	}

	// Lake naming — runs last so any cells that became seats during
	// transformations are excluded from the BFS. Each connected cluster
	// of RegionLake cells gets one name, seeded from the cluster's lex-
	// smallest cell. A lake fragmented by a settlement (rare; happens
	// when a Tributary sits on a lake-cell river bend) yields two
	// names — geologically that's now two lakes.
	{
		lakeAt := make(map[[2]int]bool)
		for _, rc := range w.Regions {
			if rc.RegionID == RegionLake {
				lakeAt[[2]int{int(rc.X), int(rc.Y)}] = true
			}
		}
		var keys [][2]int
		for k := range lakeAt {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][1] != keys[j][1] {
				return keys[i][1] < keys[j][1]
			}
			return keys[i][0] < keys[j][0]
		})
		visited := make(map[[2]int]bool)
		var nextID int64 = 1
		for _, start := range keys {
			if visited[start] {
				continue
			}
			rep := start
			queue := [][2]int{start}
			visited[start] = true
			for len(queue) > 0 {
				h := queue[0]
				queue = queue[1:]
				if h[1] < rep[1] || (h[1] == rep[1] && h[0] < rep[0]) {
					rep = h
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						n := [2]int{h[0] + dx, h[1] + dy}
						if lakeAt[n] && !visited[n] {
							visited[n] = true
							queue = append(queue, n)
						}
					}
				}
			}
			w.Lakes = append(w.Lakes, LakeInfo{
				ID:   nextID,
				Name: generateName(nameSeedForCell(seed, int64(rep[0]), int64(rep[1]))),
				X:    int64(rep[0]),
				Y:    int64(rep[1]),
			})
			nextID++
		}
	}

	return w
}

// classify is the climate→surface mapper. Order of precedence:
//  1. Agraria shelf gets a "is exposed?" check first — when its
//     elevation is at or above sea level, it always reads as Agraria,
//     regardless of temperature. (Lore: temperate microclimate; the
//     Coastals lived there during glacial peaks, so it can't be ice.)
//  2. Glaciation, where the zone allows it. Glacier outranks
//     submerged-water — a frozen sea surface reads as glacier (ice
//     shelf), not sea.
//  3. Submerged water, mapped to whichever sea/basin the zone is in.
//  4. Otherwise the zone's exposed-land identity.
func classify(b BedrockCell, lat float64, climate ClimateState) int64 {
	seaLevel := climate.SeaLevelDelta

	// Shelf cells: when exposed they always read as Agraria (their
	// lore identity) regardless of temperature; when submerged they
	// stay as Brine (no "sea ice" intermediate — keeps the
	// emerge/submerge transition visually clean).
	if b.Zone == BZAgrariaShelf {
		if b.Elevation >= seaLevel {
			return RegionAgraria
		}
		return RegionBrine
	}
	if b.Zone == BZAgrariaUpland {
		if b.Elevation >= seaLevel {
			return RegionAgrariaUpland
		}
		return RegionBrine
	}

	if canGlaciate(b.Zone) {
		if Temperature(lat, b.Elevation, climate) < glacierThreshold {
			return RegionGlacier
		}
	}

	// Note: cliff zone classification happens in bedrockZone now (it's a
	// bedrock property, not a climate one). Code retained here in case
	// future climate effects need to know cliff vs mountain.
	if b.Elevation < seaLevel {
		switch b.Zone {
		case BZBrineDeep, BZAgrariaShelf, BZAgrariaUpland:
			return RegionBrine
		case BZEastBasin:
			return RegionEastSea
		default:
			// land zones aren't normally below sea level
			return RegionEastSea
		}
	}

	switch b.Zone {
	case BZPlateau:
		return RegionPlateau
	case BZMountain:
		return RegionMountain
	case BZCliff:
		return RegionCliff
	case BZFoothill:
		return RegionFoothill
	case BZDoab:
		return RegionDoab
	case BZCradle:
		return RegionCradle
	case BZAgrariaShelf:
		return RegionAgraria
	case BZAgrariaUpland:
		return RegionAgrariaUpland
	case BZEastBasin:
		// Exposed (e.g., extreme low-stand) — reads as cradle-ish land.
		return RegionCradle
	case BZBrineDeep:
		// Should not normally happen; deep basin shouldn't be exposed.
		return RegionUnknown
	}
	return 0
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
		out[y] = clamp(52+jitter, 50, 56)
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

