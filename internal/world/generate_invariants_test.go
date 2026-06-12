package world

import (
	"fmt"
	"testing"
)

// TestGenerate_Invariants checks structural properties that must hold
// for any seed and any kya — complementary to the snapshot test, which
// pins exact output for a few cases. When worldgen changes
// intentionally, the snapshot hashes get updated; these invariants
// should never need to.
func TestGenerate_Invariants(t *testing.T) {
	cases := []struct {
		seed int64
		kya  int
	}{
		{42, KyaNow},
		{42, KyaOldWorld},
		{42, 100},
		{7, 60},
		{1234567890, KyaNow},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("seed=%d/kya=%d", c.seed, c.kya), func(t *testing.T) {
			w := Generate(c.seed, c.kya)
			checkRegionCells(t, w)
			checkRivers(t, w)
			checkSeats(t, w)
			checkFeatures(t, w)
			checkRoads(t, w)
			checkPolity(t, w)
		})
	}
}

func checkRegionCells(t *testing.T, w World) {
	t.Helper()
	seen := make(map[[2]int64]bool, len(w.Regions))
	for _, rc := range w.Regions {
		if !inBounds(int(rc.X), int(rc.Y)) {
			t.Errorf("region cell (%d,%d) out of bounds", rc.X, rc.Y)
		}
		if RegionKind(rc.RegionID) == "" {
			t.Errorf("region cell (%d,%d) has unknown RegionID %d", rc.X, rc.Y, rc.RegionID)
		}
		key := [2]int64{rc.X, rc.Y}
		if seen[key] {
			t.Errorf("duplicate region cell at (%d,%d)", rc.X, rc.Y)
		}
		seen[key] = true
	}
}

func checkRivers(t *testing.T, w World) {
	t.Helper()
	ids := make(map[int64]bool, len(w.RiverInfo))
	for _, ri := range w.RiverInfo {
		if ids[ri.ID] {
			t.Errorf("duplicate river ID %d", ri.ID)
		}
		ids[ri.ID] = true
		if ri.Name == "" {
			t.Errorf("river %d has no name", ri.ID)
		}
		if ri.Drainage < 1 {
			t.Errorf("river %d drainage = %d, want >= 1 (counts itself)", ri.ID, ri.Drainage)
		}
	}
	cellKey := make(map[[3]int64]bool, len(w.Rivers))
	for _, rc := range w.Rivers {
		if !ids[rc.RiverID] {
			t.Errorf("river cell (%d,%d) references unknown river %d", rc.X, rc.Y, rc.RiverID)
		}
		k := [3]int64{rc.RiverID, rc.X, rc.Y}
		if cellKey[k] {
			t.Errorf("duplicate river cell (%d,%d) on river %d", rc.X, rc.Y, rc.RiverID)
		}
		cellKey[k] = true
	}
	// At the glacial peak there are no rivers at all.
	if GlacialIndex(w.Kya) >= 1.0 && len(w.Rivers) != 0 {
		t.Errorf("gI=1 world has %d river cells, want 0 (water locked in ice)", len(w.Rivers))
	}
}

func checkSeats(t *testing.T, w World) {
	t.Helper()
	g := gridOf(w.Regions)
	coords := make(map[[2]int64]bool, len(w.Seats))
	for _, s := range w.Seats {
		key := [2]int64{s.X, s.Y}
		if coords[key] {
			t.Errorf("two seats share cell (%d,%d)", s.X, s.Y)
		}
		coords[key] = true
		if s.Name == "" {
			t.Errorf("seat at (%d,%d) has no name", s.X, s.Y)
		}
		switch s.Tier {
		case RegionSeat, RegionMarch, RegionHeadwater, RegionOuthold, RegionReach, RegionCapital:
		default:
			t.Errorf("seat at (%d,%d) has non-seat tier %d", s.X, s.Y, s.Tier)
		}
		// The map cell must carry the seat's tier as its region.
		if got := g.regionAt([2]int{int(s.X), int(s.Y)}); got != s.Tier {
			t.Errorf("seat at (%d,%d): cell region %d != tier %d", s.X, s.Y, got, s.Tier)
		}
		if s.Pressure < 0 || s.Pressure > 12 {
			t.Errorf("seat at (%d,%d): pressure %g out of [0,12]", s.X, s.Y, s.Pressure)
		}
	}
	// River cells never overlap seats — placeSeats filters them so the
	// seat glyph isn't painted over.
	for _, rc := range w.Rivers {
		if coords[[2]int64{rc.X, rc.Y}] {
			t.Errorf("river cell at (%d,%d) overlaps a seat", rc.X, rc.Y)
		}
	}
}

func checkFeatures(t *testing.T, w World) {
	t.Helper()
	g := gridOf(w.Regions)
	requireRegion := func(label string, x, y, want int64) {
		if got := g.regionAt([2]int{int(x), int(y)}); got != want {
			t.Errorf("%s at (%d,%d): cell region %d, want %d", label, x, y, got, want)
		}
	}
	for _, d := range w.Dens {
		requireRegion("den", d.X, d.Y, RegionDragonDen)
	}
	for _, n := range w.Nests {
		requireRegion("nest", n.X, n.Y, RegionDrakeNest)
	}
	for _, r := range w.Rookeries {
		requireRegion("rookery", r.X, r.Y, RegionWyvernRookery)
	}
	for _, p := range w.Passes {
		requireRegion("pass", p.X, p.Y, RegionPass)
	}
	for _, l := range w.Lakes {
		requireRegion("lake", l.X, l.Y, RegionLake)
		// Bathymetry: the surface is the basin's spill level, so it
		// must sit above the representative cell's bedrock, and a
		// detected lake is at least 1m deep (lakeMinDepth).
		if l.MaxDepth < 1.0 {
			t.Errorf("lake %q at (%d,%d): max depth %.2fm < 1m detection floor", l.Name, l.X, l.Y, l.MaxDepth)
		}
		if cellElev := g.elevAt([2]int{int(l.X), int(l.Y)}); l.SurfaceElev < cellElev {
			t.Errorf("lake %q at (%d,%d): surface %.1fm below its own bedrock %.1fm", l.Name, l.X, l.Y, l.SurfaceElev, cellElev)
		}
	}
	// The lake carries the river: no river cell may sit on a lake.
	lakeCells := make(map[[2]int64]bool)
	for _, rc := range w.Regions {
		if rc.RegionID == RegionLake {
			lakeCells[[2]int64{rc.X, rc.Y}] = true
		}
	}
	for _, rc := range w.Rivers {
		if lakeCells[[2]int64{rc.X, rc.Y}] {
			t.Errorf("river cell at (%d,%d) overlaps a lake — applyLakes should have absorbed it", rc.X, rc.Y)
		}
	}
	// Counts of flagged cells must match the named-feature lists.
	var passCells, denCells int
	for _, rc := range w.Regions {
		switch rc.RegionID {
		case RegionPass:
			passCells++
		case RegionDragonDen:
			denCells++
		}
	}
	if passCells != len(w.Passes) {
		t.Errorf("%d pass cells but %d PassInfo entries", passCells, len(w.Passes))
	}
	if denCells != len(w.Dens) {
		t.Errorf("%d den cells but %d DenInfo entries", denCells, len(w.Dens))
	}
}

func checkRoads(t *testing.T, w World) {
	t.Helper()
	g := gridOf(w.Regions)
	cellsByRoad := make(map[int64][]RoadCell)
	for _, rc := range w.RoadCells {
		cellsByRoad[rc.RoadID] = append(cellsByRoad[rc.RoadID], rc)
	}
	for _, r := range w.Roads {
		cells := cellsByRoad[r.ID]
		if len(cells) < 2 {
			t.Errorf("road %d has %d cells, want >= 2", r.ID, len(cells))
			continue
		}
		// Ord must be 1..n in emission order, each step 8-adjacent.
		for i, rc := range cells {
			if rc.Ord != int64(i+1) {
				t.Errorf("road %d cell %d has ord %d, want %d", r.ID, i, rc.Ord, i+1)
			}
			if i > 0 {
				dx, dy := rc.X-cells[i-1].X, rc.Y-cells[i-1].Y
				if dx < -1 || dx > 1 || dy < -1 || dy > 1 || (dx == 0 && dy == 0) {
					t.Errorf("road %d: cells %d→%d not adjacent: (%d,%d)→(%d,%d)",
						r.ID, i-1, i, cells[i-1].X, cells[i-1].Y, rc.X, rc.Y)
				}
			}
		}
		first, last := cells[0], cells[len(cells)-1]
		if first.X != r.FromX || first.Y != r.FromY {
			t.Errorf("road %d starts at (%d,%d), want From (%d,%d)", r.ID, first.X, first.Y, r.FromX, r.FromY)
		}
		if last.X != r.ToX || last.Y != r.ToY {
			t.Errorf("road %d ends at (%d,%d), want To (%d,%d)", r.ID, last.X, last.Y, r.ToX, r.ToY)
		}
		// Every road terminates at a Tributary — or the capital, which
		// was a Tributary when the roads were built and got promoted
		// by the polity layer afterward.
		switch got := g.regionAt([2]int{int(r.ToX), int(r.ToY)}); got {
		case RegionSeat, RegionCapital:
		default:
			t.Errorf("road %d terminates on region %d, want Tributary or Capital", r.ID, got)
		}
	}
	for id := range cellsByRoad {
		found := false
		for _, r := range w.Roads {
			if r.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("road cells reference unknown road %d", id)
		}
	}
}

func checkPolity(t *testing.T, w World) {
	t.Helper()
	realmByID := make(map[int64]Realm, len(w.Realms))
	var crowns, capitals int
	for _, r := range w.Realms {
		if _, dup := realmByID[r.ID]; dup {
			t.Errorf("duplicate realm ID %d", r.ID)
		}
		realmByID[r.ID] = r
		if r.Name == "" {
			t.Errorf("realm %d has no name", r.ID)
		}
		if r.IsCrown {
			crowns++
		}
	}
	if crowns > 1 {
		t.Errorf("%d crown realms, want at most 1", crowns)
	}

	seatsPerRealm := make(map[int64]int)
	for _, s := range w.Seats {
		if s.Allegiance < 0 || s.Allegiance > 1 {
			t.Errorf("seat %q allegiance %g out of [0,1]", s.Name, s.Allegiance)
		}
		if s.Tier == RegionCapital {
			capitals++
			if s.Allegiance != 1 {
				t.Errorf("capital %q allegiance %g, want 1", s.Name, s.Allegiance)
			}
			if r := realmByID[s.RealmID]; !r.IsCrown {
				t.Errorf("capital %q belongs to non-crown realm %d", s.Name, s.RealmID)
			}
		}
		// Every seat belongs to exactly one existing realm (when any
		// realms exist at all).
		if len(w.Realms) > 0 {
			if _, ok := realmByID[s.RealmID]; !ok {
				t.Errorf("seat %q references unknown realm %d", s.Name, s.RealmID)
			} else {
				seatsPerRealm[s.RealmID]++
			}
		}
	}
	if capitals > 1 {
		t.Errorf("%d capitals, want at most 1", capitals)
	}
	// A crown can only exist when a capital does.
	if crowns == 1 && capitals == 0 {
		t.Error("crown realm exists without a capital")
	}
	// Tributaries imply a capital (chooseCapital promotes one whenever
	// any Tributary exists). Note: at the LGM there are no rivers, no
	// Tributaries, and therefore no crown — the political climate
	// coupling this layer exists for.
	var tribs int
	for _, s := range w.Seats {
		if s.Tier == RegionSeat {
			tribs++
		}
	}
	if w.Kya == KyaOldWorld && capitals != 0 {
		t.Error("LGM world has a capital — the crown should not survive the ice")
	}

	for _, r := range w.Realms {
		if seatsPerRealm[r.ID] == 0 {
			t.Errorf("realm %q (%d) has no seats", r.Name, r.ID)
		}
	}

	// Territory references real realms, sits on real land cells, and
	// has no duplicates.
	g := gridOf(w.Regions)
	seen := make(map[[2]int64]bool, len(w.Territory))
	for _, tc := range w.Territory {
		if _, ok := realmByID[tc.RealmID]; !ok {
			t.Errorf("territory at (%d,%d) references unknown realm %d", tc.X, tc.Y, tc.RealmID)
		}
		k := [2]int64{tc.X, tc.Y}
		if seen[k] {
			t.Errorf("duplicate territory claim at (%d,%d)", tc.X, tc.Y)
		}
		seen[k] = true
		if g.regionAt([2]int{int(tc.X), int(tc.Y)}) == 0 {
			t.Errorf("territory at (%d,%d) claims a cell with no region", tc.X, tc.Y)
		}
	}
	// Every seat with a realm sits inside its own realm's territory.
	territoryOf := make(map[[2]int64]int64, len(w.Territory))
	for _, tc := range w.Territory {
		territoryOf[[2]int64{tc.X, tc.Y}] = tc.RealmID
	}
	for _, s := range w.Seats {
		if s.RealmID == 0 {
			continue
		}
		if got := territoryOf[[2]int64{s.X, s.Y}]; got != s.RealmID {
			t.Errorf("seat %q at (%d,%d): cell claimed by realm %d, seat belongs to %d",
				s.Name, s.X, s.Y, got, s.RealmID)
		}
	}
}

// TestDrainage_RiversSitOnTrunks cross-pins the persisted drainage
// against the river layer: every river cell rests on a region cell
// whose drainage meets the river threshold, and the map has real
// trunk drainage somewhere.
func TestDrainage_RiversSitOnTrunks(t *testing.T) {
	w := Generate(42, 0)
	drainAt := make(map[[2]int64]int64, len(w.Regions))
	var maxDrain int64
	for _, rc := range w.Regions {
		drainAt[[2]int64{rc.X, rc.Y}] = rc.Drainage
		if rc.Drainage > maxDrain {
			maxDrain = rc.Drainage
		}
	}
	thr := int64(riverThreshold())
	for _, r := range w.Rivers {
		if d := drainAt[[2]int64{r.X, r.Y}]; d < thr {
			t.Errorf("river cell (%d,%d) sits on drainage %d < threshold %d", r.X, r.Y, d, thr)
		}
	}
	if maxDrain < thr*4 {
		t.Errorf("max drainage %d — no trunk rivers? (threshold %d)", maxDrain, thr)
	}
}
