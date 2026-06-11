package world

import "github.com/hunterjsb/xan-world-sim/internal/pqueue"

// Dynamic borders. The generator's claimTerritory is the equilibrium:
// every hall projects the same influence and the cheapest claim wins.
// Inside a running slice the borders live: each hall's reach scales
// with its conviction (a sworn crown hall projects further; a league
// hall drifting toward the crown barely projects at all) and with the
// fortunes of war (the leading side pushes the front, the trailing
// side gives ground). Cells where two realms' effective claims nearly
// tie are contested marchland. The generator's function is untouched —
// deep time keeps its equilibrium; only the sim re-claims with
// weights, on a cadence (borderRefreshYears) and after every
// structural change.

const (
	// borderRefreshYears is how often the living borders re-settle
	// when nothing structural forces it sooner.
	borderRefreshYears = 5

	// A hall's influence weight: convictionFloor + convictionSpan ×
	// conviction, conviction being allegiance for crown halls and its
	// complement for independents. Range 0.7–1.3 before war pushes.
	convictionFloor = 0.7
	convictionSpan  = 0.6

	// warPush bends the front: the war's leader (|score| > warPushAt)
	// projects harder, the trailer weaker.
	warPush   = 0.08
	warPushAt = 0.2

	// contestedMargin: a runner-up realm whose effective claim is
	// within this factor of the winner's makes the cell contested.
	contestedMargin = 1.10
)

// buildCostGrid precomputes the logistic cost of entering each cell
// (river 1, road 2, terrain table otherwise; -1 impassable). Terrain,
// rivers, and roads are static within a slice; ruined or founded
// halls patch their single cell via setRegion.
func (s *Sim) buildCostGrid() {
	g := gridOf(s.W.Regions)
	roadAt := make(map[[2]int64]bool, len(s.W.RoadCells))
	for _, rc := range s.W.RoadCells {
		roadAt[[2]int64{rc.X, rc.Y}] = true
	}
	s.costGrid = make([][]int, Height)
	for y := 0; y < Height; y++ {
		s.costGrid[y] = make([]int, Width)
		for x := 0; x < Width; x++ {
			p := [2]int64{int64(x), int64(y)}
			switch {
			case s.riverAt[p]:
				s.costGrid[y][x] = 1
			case roadAt[p]:
				s.costGrid[y][x] = 2
			default:
				s.costGrid[y][x] = travelCostFor(g.regionAt([2]int{x, y}))
			}
		}
	}
}

// patchCostGrid updates one cell after setRegion (rivers and roads
// outrank terrain, exactly as buildCostGrid).
func (s *Sim) patchCostGrid(x, y, regionID int64) {
	if s.costGrid == nil {
		return
	}
	if s.riverAt[[2]int64{x, y}] {
		s.costGrid[y][x] = 1
		return
	}
	s.costGrid[y][x] = travelCostFor(regionID)
	// Roads are rare enough to scan only on the patched cell.
	for _, rc := range s.W.RoadCells {
		if rc.X == x && rc.Y == y {
			s.costGrid[y][x] = 2
			return
		}
	}
}

// seatField is a single-source Dijkstra over the cost grid, pruned at
// maxDist — the hall's claims can't reach further, so neither does
// the search (this keeps a border refresh at TUI-tick cost; the
// unpruned version cost ~2ms per hall). The shared buffer is reused
// across calls; cells beyond reach hold inf.
const fieldInf = 1 << 30

func (s *Sim) seatField(sx, sy, maxDist int) [][]int {
	if s.fieldBuf == nil {
		s.fieldBuf = make([][]int, Height)
		for y := range s.fieldBuf {
			s.fieldBuf[y] = make([]int, Width)
		}
	}
	dist := s.fieldBuf
	for y := range dist {
		for x := range dist[y] {
			dist[y][x] = fieldInf
		}
	}
	dist[sy][sx] = 0
	pq := pqueue.New(roadItemLess)
	pq.Push(roadItem{X: sx, Y: sy, Dist: 0})
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
			c := s.costGrid[ny][nx]
			if c < 0 {
				continue
			}
			nd := cur.Dist + c
			if nd > maxDist {
				continue
			}
			if nd < dist[ny][nx] {
				dist[ny][nx] = nd
				pq.Push(roadItem{X: nx, Y: ny, Dist: nd})
			}
		}
	}
	return dist
}

// seatInfluence is hall i's projection weight this year.
func (s *Sim) seatInfluence(i int) float64 {
	st := s.W.Seats[i]
	conviction := st.Allegiance
	if s.crownID == 0 || st.RealmID != s.crownID {
		conviction = 1 - st.Allegiance
	}
	wgt := convictionFloor + convictionSpan*conviction
	for _, w := range s.wars {
		var sign float64
		switch st.RealmID {
		case w.A:
			sign = 1
		case w.B:
			sign = -1
		default:
			continue
		}
		if w.Score*sign > warPushAt {
			wgt += warPush
		} else if w.Score*sign < -warPushAt {
			wgt -= warPush
		}
	}
	return min(max(wgt, 0.5), 1.45)
}

// reclaimTerritory re-settles the living borders: every cell goes to
// the realm whose hall reaches it cheapest in *effective* distance
// (true distance / influence), out to claimRadius effective. Cells
// where another realm's claim lands within contestedMargin are
// contested marchland. Ties go to the earlier hall in seat order.
func (s *Sim) reclaimTerritory() {
	s.terrVersion++
	s.W.Territory = s.W.Territory[:0]
	s.contested = make(map[[2]int64]bool)
	if len(s.W.Seats) == 0 {
		return
	}
	type cellClaim struct {
		best      float64
		bestRealm int64
		runner    float64 // best effective claim by a different realm
	}
	claims := make([][]cellClaim, Height)
	for y := range claims {
		claims[y] = make([]cellClaim, Width)
		for x := range claims[y] {
			claims[y][x] = cellClaim{best: -1, runner: -1}
		}
	}
	for i := range s.W.Seats {
		st := s.W.Seats[i]
		if st.RealmID == 0 {
			continue
		}
		weight := s.seatInfluence(i)
		// The search needs nothing past the hall's own claim reach,
		// stretched by the contested margin so runner-up claims that
		// could mark a cell contested are still seen.
		maxDist := int(claimRadius*weight*contestedMargin) + 1
		field := s.seatField(int(st.X), int(st.Y), maxDist)
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				d := field[y][x]
				if d >= fieldInf {
					continue
				}
				eff := float64(d) / weight
				// Track up to the contested margin past the claim
				// radius: such reaches can't own a cell but can make
				// a neighbor's ownership contested.
				if eff > claimRadius*contestedMargin {
					continue
				}
				c := &claims[y][x]
				switch {
				case c.best < 0 || eff < c.best:
					if c.bestRealm != 0 && c.bestRealm != st.RealmID {
						c.runner = c.best
					}
					c.best, c.bestRealm = eff, st.RealmID
				case st.RealmID != c.bestRealm && (c.runner < 0 || eff < c.runner):
					c.runner = eff
				}
			}
		}
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			c := claims[y][x]
			if c.bestRealm == 0 || c.best > claimRadius {
				continue // unclaimed, or only over-the-margin reaches
			}
			s.W.Territory = append(s.W.Territory, TerritoryCell{
				X: int64(x), Y: int64(y), RealmID: c.bestRealm,
			})
			if c.runner >= 0 && c.runner <= c.best*contestedMargin {
				s.contested[[2]int64{int64(x), int64(y)}] = true
			}
		}
	}
}

// Contested reports whether the cell is contested marchland.
func (s *Sim) Contested(x, y int64) bool { return s.contested[[2]int64{x, y}] }

// TerritoryVersion increments whenever the borders re-settle; the TUI
// compares it to know when to rebuild its territory rows.
func (s *Sim) TerritoryVersion() int { return s.terrVersion }
