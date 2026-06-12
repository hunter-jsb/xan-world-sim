package world

import "testing"

// TestGeology_LithologyTotal: every cell carries a known rock laid at
// a sane moment — the geological lens must never find a blank.
func TestGeology_LithologyTotal(t *testing.T) {
	for _, kya := range []int{KyaNow, KyaOldWorld} {
		w := Generate(42, kya)
		for _, rc := range w.Regions {
			if RockKind(rc.Rock) == "" {
				t.Fatalf("kya=%d: cell (%d,%d) has unknown rock %d", kya, rc.X, rc.Y, rc.Rock)
			}
			if rc.RockAge < 0 || rc.RockAge > geoStart {
				t.Fatalf("kya=%d: cell (%d,%d) rock age %d outside [0,%d]", kya, rc.X, rc.Y, rc.RockAge, geoStart)
			}
		}
	}
}

// TestGeology_VolcanismFires: the rift has live vents — every seed
// rolls a roster, every vent has spoken by the present, summits are
// stamped, and somewhere the rock is volcanic.
func TestGeology_VolcanismFires(t *testing.T) {
	for _, seed := range []int64{0, 7, 42} {
		w := Generate(seed, KyaNow)
		if n := len(w.Volcanoes); n < volcanoMin || n >= volcanoMin+volcanoExtra {
			t.Errorf("seed=%d: %d volcanoes, want %d..%d", seed, n, volcanoMin, volcanoMin+volcanoExtra-1)
		}
		g := gridOf(w.Regions)
		var lavaCells, volcanoCells int
		for _, rc := range w.Regions {
			if rc.Rock == RockLava {
				lavaCells++
			}
			if rc.RegionID == RegionVolcano {
				volcanoCells++
			}
		}
		if volcanoCells != len(w.Volcanoes) {
			t.Errorf("seed=%d: %d volcano cells but %d VolcanoInfo entries", seed, volcanoCells, len(w.Volcanoes))
		}
		for _, v := range w.Volcanoes {
			if v.Name == "" {
				t.Errorf("seed=%d: volcano %d has no name", seed, v.ID)
			}
			if v.Eruptions < 1 {
				t.Errorf("seed=%d: volcano %q exists with %d eruptions — birth requires one", seed, v.Name, v.Eruptions)
			}
			if v.LastAgo < 0 {
				t.Errorf("seed=%d: volcano %q last eruption %d ka ago — the future leaked", seed, v.Name, v.LastAgo)
			}
			if got := g.regionAt([2]int{int(v.X), int(v.Y)}); got != RegionVolcano {
				t.Errorf("seed=%d: volcano %q summit cell is region %d, want volcano", seed, v.Name, got)
			}
		}
		// Over 600 ka with eruptions at most eruptGapMax apart, the
		// world must carry volcanic rock somewhere.
		if lavaCells == 0 {
			t.Errorf("seed=%d: no volcanic rock anywhere after %d ka of history", seed, geoStart)
		}
	}
}

// TestGeology_VolcanoesAreBornInOrder: a vent's eruption count only
// grows as time runs forward (kya shrinks), and the roster never
// shrinks — scrubbing toward the present, volcanoes are born and
// build, never unhappen.
func TestGeology_VolcanoesAreBornInOrder(t *testing.T) {
	prev := map[[2]int64]int64{}
	for _, kya := range []int{300, 205, 100, 0} {
		w := Generate(42, kya)
		for _, v := range w.Volcanoes {
			k := [2]int64{v.X, v.Y}
			if v.Eruptions < prev[k] {
				t.Errorf("volcano at (%d,%d): eruptions fell %d→%d moving 300→%d kya",
					v.X, v.Y, prev[k], v.Eruptions, kya)
			}
			prev[k] = v.Eruptions
		}
	}
}

// TestGeology_IceLeavesItsMark: at the warm present the deglaciated
// north reads till with a loess belt at its fringe; at the glacial
// peak the ice is still sitting on its load, so till is scarce and
// the periglacial steppe is deep in dust.
func TestGeology_IceLeavesItsMark(t *testing.T) {
	count := func(w World, rock int64) int {
		n := 0
		for _, rc := range w.Regions {
			if rc.Rock == rock {
				n++
			}
		}
		return n
	}
	now := Generate(42, KyaNow)
	lgm := Generate(42, KyaOldWorld)
	if tillNow := count(now, RockTill); tillNow < 100 {
		t.Errorf("present-day till on %d cells — deglaciation should blanket the north", tillNow)
	}
	if loessNow := count(now, RockLoess); loessNow < 50 {
		t.Errorf("present-day loess on %d cells — the steppe belt is missing", loessNow)
	}
	tillLGM, tillNow := count(lgm, RockTill), count(now, RockTill)
	if tillLGM >= tillNow {
		t.Errorf("LGM till (%d) >= present till (%d) — the ice should still be sitting on its load", tillLGM, tillNow)
	}
}

// TestGeology_IsostasyAtLGM: under the LGM ice the crust rides lower
// than the same cells today — depression at the cold peak, rebound by
// the warm present.
func TestGeology_IsostasyAtLGM(t *testing.T) {
	now := Generate(42, KyaNow)
	lgm := Generate(42, KyaOldWorld)
	elevNow := make(map[[2]int64]float64, len(now.Regions))
	for _, rc := range now.Regions {
		elevNow[[2]int64{rc.X, rc.Y}] = rc.Elevation
	}
	gLGM := gridOf(lgm.Regions)
	var depressed int
	for _, rc := range lgm.Regions {
		if gLGM.regionAt([2]int{int(rc.X), int(rc.Y)}) != RegionGlacier {
			continue
		}
		if rc.Elevation < elevNow[[2]int64{rc.X, rc.Y}]-30 {
			depressed++
		}
	}
	if depressed < 50 {
		t.Errorf("only %d LGM-glaciated cells ride >30m lower than today — isostasy isn't pressing", depressed)
	}
}

// TestGeology_ScrubCoherence: adjacent deep-time stops are the same
// world five thousand years apart, not two unrelated rolls — bounded
// lithology churn, no elevation jumps outside an eruption's reach.
func TestGeology_ScrubCoherence(t *testing.T) {
	for _, pair := range [][2]int{{5, 0}, {105, 100}, {205, 200}} {
		a := Generate(42, pair[0])
		b := Generate(42, pair[1])
		cellsA := make(map[[2]int64]RegionCell, len(a.Regions))
		for _, rc := range a.Regions {
			cellsA[[2]int64{rc.X, rc.Y}] = rc
		}
		changedRock, bigElev := 0, 0
		for _, rc := range b.Regions {
			pa := cellsA[[2]int64{rc.X, rc.Y}]
			if pa.Rock != rc.Rock {
				changedRock++
			}
			if d := rc.Elevation - pa.Elevation; d > 60 || d < -60 {
				bigElev++
			}
		}
		if changedRock > len(b.Regions)/10 {
			t.Errorf("kya %d→%d: rock changed on %d/%d cells — one epoch shouldn't repaint the map",
				pair[0], pair[1], changedRock, len(b.Regions))
		}
		if bigElev > 30 {
			t.Errorf("kya %d→%d: %d cells moved >60m in one epoch — beyond any cone's reach", pair[0], pair[1], bigElev)
		}
	}
}

// TestGeology_FreshLavaIsHostile: every lava-field cell is fresh
// volcanic rock, costly to cross, and never built on.
func TestGeology_FreshLavaIsHostile(t *testing.T) {
	for _, kya := range []int{KyaNow, 100} {
		w := Generate(42, kya)
		seatAt := make(map[[2]int64]bool, len(w.Seats))
		for _, s := range w.Seats {
			seatAt[[2]int64{s.X, s.Y}] = true
		}
		for _, rc := range w.Regions {
			if rc.RegionID != RegionLava {
				continue
			}
			if rc.Rock != RockLava || rc.RockAge > lavaFreshKa {
				t.Errorf("kya=%d: lava field at (%d,%d) carries %s aged %d — the region should have healed",
					kya, rc.X, rc.Y, RockKind(rc.Rock), rc.RockAge)
			}
			if seatAt[[2]int64{rc.X, rc.Y}] {
				t.Errorf("kya=%d: a seat stands on fresh lava at (%d,%d)", kya, rc.X, rc.Y)
			}
		}
	}
}
