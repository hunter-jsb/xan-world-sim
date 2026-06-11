package world

import "github.com/hunterjsb/xan-world-sim/internal/pqueue"

// The polity layer derives the political map from geography, per the
// lore in region.md: "a wealthy, populous downstream heartland
// (capital, agriculture, trade) and a ring of rugged upstream marches
// (battle-hardened, independent-minded)... Geography itself makes them
// harder to control and easier to rebel."
//
// Everything is computed, nothing is declared: the capital is where
// drainage and calm converge; allegiance is logistics; realms are what
// allegiance and valley-distance leave behind. Because rivers are
// climate-coupled, so is the whole political map — scrub toward the
// LGM and the crown's river-borne reach collapses until only scattered
// March enclaves remain.

// allegianceLambda is the logistic cost at which crown control halves
// (allegiance = 1/(1 + L/λ)). Grounded as Width+Height: the cheapest
// (all-river, cost 1/cell) crossing of the whole map. A seat at the
// far end of the river network keeps allegiance ≈ 0.5 (tributary
// grade); the same distance overland costs ~4× more and decays to
// ≈ 0.2 (autonomous) — "the river is the crown's reach."
const allegianceLambda = float64(Width + Height)

// enclaveRadius is the maximum seat-to-seat logistic cost for two
// independent seats to belong to the same enclave. Grounded at the
// valley scale: 5 cells of open land (cost 4/cell ≈ 250km) —
// neighbors a hall can reach within days. "Each enclave is somewhat
// isolated from its neighbors" beyond that; chains of mutual
// reachability still bind a river valley into one league.
const enclaveRadius = 20

// claimRadius is how far (in logistic cost) a hall's sphere of
// control extends for territory claims. Grounded at the dragon-raid
// scale the lore uses for patrol reach: 12 cells of open land
// (cost 4/cell ≈ 600km) — what a seat can garrison and answer for.
// Beyond every seat's claimRadius the land is unclaimed wilds.
const claimRadius = 48

// crownThreshold is the allegiance at or above which a seat is part
// of the Crown realm rather than an independent enclave — the
// boundary between the "tributary" and "nominal" stances.
const crownThreshold = 0.5

// AllegianceStance labels an allegiance score with the lore's
// vocabulary for how a seat currently relates to the crown.
func AllegianceStance(a float64) string {
	switch {
	case a >= 0.75:
		return "sworn"
	case a >= crownThreshold:
		return "tributary"
	case a >= 0.25:
		return "nominal"
	default:
		return "autonomous"
	}
}

// logisticCostFrom runs a Dijkstra over the political travel graph
// (rivers 1, roads 2, otherwise the terrain cost table) from the given
// source cells and returns per-cell total cost (-1 = unreachable).
// This is the crown-courier metric: control flows down rivers first,
// roads second, open land grudgingly, and never across mountains.
func (w *World) logisticCostFrom(sources [][2]int) [][]int {
	g := gridOf(w.Regions)
	riverAt := make(map[[2]int]bool, len(w.Rivers))
	for _, r := range w.Rivers {
		riverAt[[2]int{int(r.X), int(r.Y)}] = true
	}
	roadAt := make(map[[2]int]bool, len(w.RoadCells))
	for _, rc := range w.RoadCells {
		roadAt[[2]int{int(rc.X), int(rc.Y)}] = true
	}
	cost := func(p [2]int) int {
		if riverAt[p] {
			return 1
		}
		if roadAt[p] {
			return 2
		}
		return travelCostFor(g.regionAt(p))
	}

	const inf = 1 << 30
	dist := make([][]int, Height)
	for y := range dist {
		dist[y] = make([]int, Width)
		for x := range dist[y] {
			dist[y][x] = inf
		}
	}
	pq := pqueue.New(roadItemLess)
	for _, s := range sources {
		dist[s[1]][s[0]] = 0
		pq.Push(roadItem{X: s[0], Y: s[1], Dist: 0})
	}
	for pq.Len() > 0 {
		cur := pq.Pop()
		if cur.Dist > dist[cur.Y][cur.X] {
			continue
		}
		for _, d := range dirs8 {
			nx, ny := cur.X+d[0], cur.Y+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			c := cost([2]int{nx, ny})
			if c < 0 {
				continue
			}
			nd := cur.Dist + c
			if nd < dist[ny][nx] {
				dist[ny][nx] = nd
				pq.Push(roadItem{X: nx, Y: ny, Dist: nd})
			}
		}
	}
	for y := range dist {
		for x := range dist[y] {
			if dist[y][x] == inf {
				dist[y][x] = -1
			}
		}
	}
	return dist
}

// chooseCapital promotes the best-positioned Tributary to the crown's
// capital. Lore rule: "larger, calmer settlements form further from
// the mountains... where the populous capitals are most likely to
// sit" — on the main river. Geographic signature: the Tributary
// adjacent to the highest-drainage river (the cradle's "Mississippi"),
// furthest downstream along it (max Ord), least dragon pressure.
// Returns the capital's index in w.Seats, or -1 when no Tributary
// exists (e.g., at the LGM there are no rivers and therefore no crown).
func (w *World) chooseCapital() int {
	drainageOf := make(map[int64]int64, len(w.RiverInfo))
	for _, ri := range w.RiverInfo {
		drainageOf[ri.ID] = ri.Drainage
	}
	type rcell struct {
		id  int64
		ord int64
	}
	riverAt := make(map[[2]int]rcell, len(w.Rivers))
	for _, r := range w.Rivers {
		riverAt[[2]int{int(r.X), int(r.Y)}] = rcell{r.RiverID, r.Ord}
	}

	best := -1
	var bestDrain, bestOrd int64
	var bestPressure float64
	for i := range w.Seats {
		s := &w.Seats[i]
		if s.Tier != RegionSeat {
			continue
		}
		// The seat sits on its (filtered-out) river cell; the chain's
		// neighbors are still in w.Rivers. Take the strongest river
		// touching the seat and the furthest-downstream ord of it.
		var drain, ord int64
		for _, d := range dirs8 {
			n := [2]int{int(s.X) + d[0], int(s.Y) + d[1]}
			rc, ok := riverAt[n]
			if !ok {
				continue
			}
			rd := drainageOf[rc.id]
			if rd > drain || (rd == drain && rc.ord > ord) {
				drain, ord = rd, rc.ord
			}
		}
		better := false
		switch {
		case best == -1:
			better = true
		case drain != bestDrain:
			better = drain > bestDrain
		case ord != bestOrd:
			better = ord > bestOrd
		case s.Pressure != bestPressure:
			better = s.Pressure < bestPressure
		}
		if better {
			best, bestDrain, bestOrd, bestPressure = i, drain, ord, s.Pressure
		}
	}
	if best < 0 {
		return -1
	}
	capSeat := &w.Seats[best]
	capSeat.Tier = RegionCapital
	for i := range w.Regions {
		rc := &w.Regions[i]
		if rc.X == capSeat.X && rc.Y == capSeat.Y {
			rc.RegionID = RegionCapital
			break
		}
	}
	return best
}

// computeAllegiance scores every seat's crown control in [0, 1]:
//
//	base     = 1 / (1 + L/λ)   — L = logistic cost from the capital
//	tier     — Tributary +0.15 (crown-subsidized, transactionally
//	           loyal), March +0.05 (duty: "we are the wall"),
//	           Headwater +0 (sacred, contested by religious orders),
//	           Outhold −0.15 (off-grid, no formal role),
//	           Reach −0.30 ("essentially autonomous in practice")
//	pressure — −0.02 per point of dragon pressure: defense demands
//	           military self-sufficiency, and self-sufficiency breeds
//	           independence.
//
// With no capital (no rivers, e.g. LGM) every seat scores 0 — there
// is no crown to be loyal to.
func (w *World) computeAllegiance(capitalIdx int) {
	if capitalIdx < 0 {
		return
	}
	capSeat := w.Seats[capitalIdx]
	dist := w.logisticCostFrom([][2]int{{int(capSeat.X), int(capSeat.Y)}})
	for i := range w.Seats {
		s := &w.Seats[i]
		if i == capitalIdx {
			s.Allegiance = 1
			continue
		}
		L := dist[s.Y][s.X]
		if L < 0 {
			s.Allegiance = 0 // unreachable: beyond the crown's world
			continue
		}
		a := 1 / (1 + float64(L)/allegianceLambda)
		switch s.Tier {
		case RegionSeat:
			a += 0.15
		case RegionMarch:
			a += 0.05
		case RegionOuthold:
			a -= 0.15
		case RegionReach:
			a -= 0.30
		}
		a -= 0.02 * s.Pressure
		s.Allegiance = min(max(a, 0), 1)
	}
}

// formRealms partitions the seats into polities. Seats at or above
// crownThreshold belong to the Crown (named for its capital). The
// rest cluster into independent enclaves by valley distance — two
// independent seats share an enclave when the logistic cost between
// them is within enclaveRadius. Each enclave is led by its eldest
// hall (March lineages are the oldest per lore, then Headwater,
// Tributary, Outhold, Reach; ties go north-west) and takes that
// hall's name.
func (w *World) formRealms(capitalIdx int) {
	if len(w.Seats) == 0 {
		return
	}
	var crown []int
	var indep []int
	for i := range w.Seats {
		if capitalIdx >= 0 && w.Seats[i].Allegiance >= crownThreshold {
			crown = append(crown, i)
		} else {
			indep = append(indep, i)
		}
	}

	var nextID int64 = 1
	if len(crown) > 0 {
		capSeat := w.Seats[capitalIdx]
		w.Realms = append(w.Realms, Realm{
			ID:      nextID,
			Name:    capSeat.Name,
			IsCrown: true,
			SeatX:   capSeat.X,
			SeatY:   capSeat.Y,
		})
		for _, i := range crown {
			w.Seats[i].RealmID = nextID
		}
		nextID++
	}

	// Enclave clustering: union seats within enclaveRadius of each
	// other. Pairwise costs via one Dijkstra per independent seat.
	parent := make([]int, len(indep))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for ii, i := range indep {
		s := w.Seats[i]
		dist := w.logisticCostFrom([][2]int{{int(s.X), int(s.Y)}})
		for jj := ii + 1; jj < len(indep); jj++ {
			t := w.Seats[indep[jj]]
			if d := dist[t.Y][t.X]; d >= 0 && d <= enclaveRadius {
				ri, rj := find(ii), find(jj)
				if ri != rj {
					if ri < rj {
						parent[rj] = ri
					} else {
						parent[ri] = rj
					}
				}
			}
		}
	}

	// tierAge orders seat tiers by lineage age for enclave leadership.
	tierAge := func(t int64) int {
		switch t {
		case RegionMarch:
			return 0 // "March lineages are the oldest"
		case RegionHeadwater:
			return 1
		case RegionSeat:
			return 2
		case RegionOuthold:
			return 3
		default: // Reach — the newest, furthest out
			return 4
		}
	}
	leaderOf := make(map[int]int) // cluster root → seat index of leader
	for ii, i := range indep {
		root := find(ii)
		s := w.Seats[i]
		cur, ok := leaderOf[root]
		if !ok {
			leaderOf[root] = i
			continue
		}
		c := w.Seats[cur]
		if tierAge(s.Tier) < tierAge(c.Tier) ||
			(tierAge(s.Tier) == tierAge(c.Tier) &&
				(s.Y < c.Y || (s.Y == c.Y && s.X < c.X))) {
			leaderOf[root] = i
		}
	}
	// Emit enclaves ordered by leader (y, x) for determinism.
	type encl struct {
		root   int
		leader int
	}
	var enclaves []encl
	seen := make(map[int]bool)
	for ii := range indep {
		root := find(ii)
		if seen[root] {
			continue
		}
		seen[root] = true
		enclaves = append(enclaves, encl{root, leaderOf[root]})
	}
	for i := 1; i < len(enclaves); i++ {
		for j := i; j > 0; j-- {
			a, b := w.Seats[enclaves[j-1].leader], w.Seats[enclaves[j].leader]
			if b.Y < a.Y || (b.Y == a.Y && b.X < a.X) {
				enclaves[j-1], enclaves[j] = enclaves[j], enclaves[j-1]
			} else {
				break
			}
		}
	}
	for _, e := range enclaves {
		lead := w.Seats[e.leader]
		w.Realms = append(w.Realms, Realm{
			ID:    nextID,
			Name:  lead.Name,
			SeatX: lead.X,
			SeatY: lead.Y,
		})
		for ii, i := range indep {
			if find(ii) == e.root {
				w.Seats[i].RealmID = nextID
			}
		}
		nextID++
	}
}

// claimTerritory assigns every reachable land cell to the realm of
// the seat that can reach it cheapest (multi-source Dijkstra with
// per-seat ownership), out to claimRadius. Beyond that the land is
// unclaimed wilds. Ownership ties resolve to the earlier-expanded
// frontier entry — deterministic via the (dist, y, x) heap order.
func (w *World) claimTerritory() {
	if len(w.Seats) == 0 {
		return
	}
	g := gridOf(w.Regions)
	riverAt := make(map[[2]int]bool, len(w.Rivers))
	for _, r := range w.Rivers {
		riverAt[[2]int{int(r.X), int(r.Y)}] = true
	}
	roadAt := make(map[[2]int]bool, len(w.RoadCells))
	for _, rc := range w.RoadCells {
		roadAt[[2]int{int(rc.X), int(rc.Y)}] = true
	}
	cost := func(p [2]int) int {
		if riverAt[p] {
			return 1
		}
		if roadAt[p] {
			return 2
		}
		return travelCostFor(g.regionAt(p))
	}

	const inf = 1 << 30
	dist := make([][]int, Height)
	owner := make([][]int64, Height) // realm ID, 0 = unclaimed
	for y := range dist {
		dist[y] = make([]int, Width)
		owner[y] = make([]int64, Width)
		for x := range dist[y] {
			dist[y][x] = inf
		}
	}
	pq := pqueue.New(roadItemLess)
	for i := range w.Seats {
		s := w.Seats[i]
		if s.RealmID == 0 {
			continue
		}
		dist[s.Y][s.X] = 0
		owner[s.Y][s.X] = s.RealmID
		pq.Push(roadItem{X: int(s.X), Y: int(s.Y), Dist: 0})
	}
	for pq.Len() > 0 {
		cur := pq.Pop()
		if cur.Dist > dist[cur.Y][cur.X] {
			continue
		}
		for _, d := range dirs8 {
			nx, ny := cur.X+d[0], cur.Y+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			c := cost([2]int{nx, ny})
			if c < 0 {
				continue
			}
			nd := cur.Dist + c
			if nd > claimRadius {
				continue
			}
			if nd < dist[ny][nx] {
				dist[ny][nx] = nd
				owner[ny][nx] = owner[cur.Y][cur.X]
				pq.Push(roadItem{X: nx, Y: ny, Dist: nd})
			}
		}
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if owner[y][x] != 0 {
				w.Territory = append(w.Territory, TerritoryCell{
					X: int64(x), Y: int64(y), RealmID: owner[y][x],
				})
			}
		}
	}
}
