package world

import (
	"context"
	"sync"
	"testing"
)

// The canonical fate of one eventful millennium, computed once and
// shared — a full slice run costs seconds. The scan insists on a
// fate with at least one fallen hall so the tell assertions bite.
var (
	fateOnce  sync.Once
	fateSeed  int64
	fateKya   int
	testFate  Fate
	fateFound bool
)

func eventfulFate(t *testing.T) (int64, int, Fate) {
	t.Helper()
	fateOnce.Do(func() {
		for _, seed := range []int64{42, 0, 7, 3} {
			f := CanonicalFate(seed, 1, nil)
			if len(f.Ruins) > 0 && len(f.Seats) > 0 {
				fateSeed, fateKya, testFate, fateFound = seed, 1, f, true
				return
			}
		}
	})
	if !fateFound {
		t.Fatal("no scanned seed loses a single hall in a millennium — the calibration has gone soft")
	}
	return fateSeed, fateKya, testFate
}

// TestFate_RolloverCarriesTheRecord: seal an age, generate the next
// step of deep time, and the world remembers — every recorded loss
// on surviving ground is a tell, every surviving hall still stands
// (or its ground visibly no longer holds it), and the fated world
// passes the same structural invariants as a pure one.
func TestFate_RolloverCarriesTheRecord(t *testing.T) {
	seed, kya, fate := eventfulFate(t)
	if len(fate.Annals) == 0 {
		t.Fatal("an eventful millennium left no annals")
	}
	w := GenerateWithFates(seed, kya-1, []Fate{fate})

	g := gridOf(w.Regions)
	tellAt := make(map[[2]int64]bool, len(w.Tells))
	for _, tl := range w.Tells {
		tellAt[[2]int64{tl.X, tl.Y}] = true
		if got := g.regionAt([2]int{int(tl.X), int(tl.Y)}); got != RegionRuin {
			t.Errorf("tell %q at (%d,%d) sits on region %d, want ruin", tl.Name, tl.X, tl.Y, got)
		}
		if tl.Story == "" || tl.EraKya != fate.Kya {
			t.Errorf("tell %q carries story %q era %d — provenance lost", tl.Name, tl.Story, tl.EraKya)
		}
	}
	seatAt := make(map[[2]int64]bool, len(w.Seats))
	for _, s := range w.Seats {
		seatAt[[2]int64{s.X, s.Y}] = true
	}
	nearSeat := func(x, y int64) bool {
		for dy := int64(-1); dy <= 1; dy++ {
			for dx := int64(-1); dx <= 1; dx++ {
				if seatAt[[2]int64{x + dx, y + dy}] {
					return true
				}
			}
		}
		return false
	}
	for _, fr := range fate.Ruins {
		p := [2]int64{fr.X, fr.Y}
		if tellAt[p] || seatAt[p] {
			continue // remembered, or built upon
		}
		if fateSeatViable(g.regionAt([2]int{int(fr.X), int(fr.Y)})) {
			t.Errorf("the fall of %s at (%d,%d) was forgotten on ground that still holds", fr.Name, fr.X, fr.Y)
		}
	}
	for _, fs := range fate.Seats {
		if nearSeat(fs.X, fs.Y) {
			continue // it stands, or the new age holds the same ground
		}
		if fateSeatViable(g.regionAt([2]int{int(fs.X), int(fs.Y)})) {
			t.Errorf("the hall of %s at (%d,%d) was lost in the fold on ground that still holds", fs.Name, fs.X, fs.Y)
		}
	}

	// The fated world is structurally as sound as a pure one.
	checkRegionCells(t, w)
	checkSeats(t, w)
	checkRoads(t, w)
	checkPolity(t, w)

	// And deterministic.
	if hashWorld(w) != hashWorld(GenerateWithFates(seed, kya-1, []Fate{fate})) {
		t.Error("the same chain generated two different worlds")
	}
}

// TestFate_HousesPersist: a slice on a fated world opens with the old
// houses in their halls and a dawn entry in the chronicle.
func TestFate_HousesPersist(t *testing.T) {
	seed, kya, fate := eventfulFate(t)
	s := NewSimWithFates(seed, kya-1, []Fate{fate})
	houseOf := make(map[[2]int64]string, len(fate.Seats))
	for _, fs := range fate.Seats {
		houseOf[[2]int64{fs.X, fs.Y}] = fs.House
	}
	carried := 0
	for i := range s.W.Seats {
		if h, ok := houseOf[[2]int64{s.W.Seats[i].X, s.W.Seats[i].Y}]; ok {
			if s.house[i] != h {
				t.Errorf("hall at (%d,%d): house %q, the old age sealed %q",
					s.W.Seats[i].X, s.W.Seats[i].Y, s.house[i], h)
			}
			if got := s.HouseTenure(s.W.Seats[i].X, s.W.Seats[i].Y); got < 1 {
				t.Errorf("hall at (%d,%d): carried house has tenure %d, want ≥ 1 sealed age",
					s.W.Seats[i].X, s.W.Seats[i].Y, got)
			}
			carried++
		}
	}
	if carried == 0 {
		t.Error("no hall carried a house across the millennium")
	}
	if len(s.Log) == 0 || s.Log[0].Kind != "epoch" || s.Log[0].Year != 0 {
		t.Error("the chronicle does not open with the dawn of the new age")
	}
	if s.ageNumber != fate.Age+1 {
		t.Errorf("the new slice counts itself age %d after sealed age %d", s.ageNumber, fate.Age)
	}
}

// TestFate_LineageTempersAndGrudges: the rest of what crosses a dawn
// — realm lines count their ages, a buried den stays buried, a
// surviving dragon wakes with the temper it held, and old enemies
// open the age with embers still warm.
func TestFate_LineageTempersAndGrudges(t *testing.T) {
	base := Generate(42, 0)
	var crown, league string
	for _, r := range base.Realms {
		if r.IsCrown {
			crown = r.Name
		} else if league == "" {
			league = r.Name
		}
	}
	if crown == "" || league == "" {
		t.Fatal("base world lacks a crown or a league — the probe needs both")
	}
	if len(base.Dens) < 2 {
		t.Fatal("base world has fewer than two dragon dens")
	}
	hot, buried := base.Dens[0], base.Dens[1]
	fate := Fate{Seed: 42, Kya: 1, Age: 3,
		Realms: []FateRealm{{Name: crown, IsCrown: true, Age: 3}},
		Lairs: []FateLair{
			{X: hot.X, Y: hot.Y, Kind: "dragon", Activity: 1.8},
			{X: buried.X, Y: buried.Y, Buried: true},
		},
		Grudges: []FateGrudge{{A: crown, B: league, Heat: 0.8}},
	}
	chain := []Fate{fate}

	w := GenerateWithFates(42, 0, chain)
	for _, d := range w.Dens {
		if d.X == buried.X && d.Y == buried.Y {
			t.Error("the buried den re-formed — the mountain gave back what it took")
		}
	}
	if got := gridOf(w.Regions).regionAt([2]int{int(buried.X), int(buried.Y)}); got != RegionMountain {
		t.Errorf("the buried den's cell is region %d, want plain mountain", got)
	}
	crownAged := false
	for _, r := range w.Realms {
		if r.Name == crown {
			crownAged = true
			if r.Age != 4 {
				t.Errorf("the crown of %s is in age %d, want 4 (3 sealed + this one)", crown, r.Age)
			}
		} else if r.Age != 1 {
			t.Errorf("realm %q has age %d with no recorded lineage", r.Name, r.Age)
		}
	}
	if !crownAged {
		t.Errorf("the crown of %s did not re-form — lineage has nothing to land on", crown)
	}

	s := NewSimWithFates(42, 0, chain)
	if s.ageNumber != 4 {
		t.Errorf("slice counts itself age %d, want 4", s.ageNumber)
	}
	if got := s.LairActivity(hot.X, hot.Y); got != 1.8 {
		t.Errorf("the dragon of %s wakes at activity %g, sealed at 1.8", hot.Name, got)
	}
	var a, b int64
	for _, r := range s.W.Realms {
		if r.Name == crown {
			a = r.ID
		}
		if r.Name == league {
			b = r.ID
		}
	}
	if a == 0 || b == 0 {
		t.Fatalf("crown %q or league %q missing from the fated slice", crown, league)
	}
	if got := s.grievance[pairKey(a, b)]; got != 0.8*grudgeEmberK {
		t.Errorf("the old feud opens at %g, want %g", got, 0.8*grudgeEmberK)
	}
}

// TestFate_NilChainIsPure: no chain, an empty chain, and a chain
// whose ages are all at-or-after this moment generate the snapshot
// world byte for byte.
func TestFate_NilChainIsPure(t *testing.T) {
	pure := hashWorld(Generate(42, 100))
	if hashWorld(GenerateWithFates(42, 100, []Fate{})) != pure {
		t.Error("an empty chain perturbed the world")
	}
	future := []Fate{{Seed: 42, Kya: 100, Ruins: []FateRuin{{X: 60, Y: 25, Name: "X", Story: "y"}}}}
	if hashWorld(GenerateWithFates(42, 100, future)) != pure {
		t.Error("an age at this very moment leaked into its own past")
	}
}

// TestFate_ReconciliationDropsTheLost: ground the new era has taken
// keeps its dead in the annals but not on the map.
func TestFate_ReconciliationDropsTheLost(t *testing.T) {
	base := Generate(42, 0)
	seatAt := make(map[[2]int64]bool, len(base.Seats))
	for _, s := range base.Seats {
		seatAt[[2]int64{s.X, s.Y}] = true
	}
	// One cell that is open water, one livable cell far from any hall.
	var wetX, wetY, dryX, dryY int64 = -1, -1, -1, -1
	for _, rc := range base.Regions {
		switch rc.RegionID {
		case RegionBrine:
			if wetX < 0 {
				wetX, wetY = rc.X, rc.Y
			}
		case RegionForest, RegionCradle:
			if dryX >= 0 {
				continue
			}
			clear := true
			for dy := int64(-2); dy <= 2 && clear; dy++ {
				for dx := int64(-2); dx <= 2; dx++ {
					if seatAt[[2]int64{rc.X + dx, rc.Y + dy}] {
						clear = false
						break
					}
				}
			}
			if clear {
				dryX, dryY = rc.X, rc.Y
			}
		}
	}
	if wetX < 0 || dryX < 0 {
		t.Fatal("could not find probe cells on the base world")
	}
	chain := []Fate{{Seed: 42, Kya: 1,
		Seats: []FateSeat{{X: wetX, Y: wetY, Tier: RegionOuthold, Name: "Drownedhold", House: "Lost"}},
		Ruins: []FateRuin{
			{X: wetX, Y: wetY, Name: "Drownedtell", Story: "taken by the sea"},
			{X: dryX, Y: dryY, Name: "Drytell", Story: "test ruin on living ground"},
		}}}
	w := GenerateWithFates(42, 0, chain)
	for _, s := range w.Seats {
		if s.X == wetX && s.Y == wetY {
			t.Error("a hall stands on open water — reconciliation failed")
		}
	}
	foundDry := false
	for _, tl := range w.Tells {
		if tl.X == wetX && tl.Y == wetY {
			t.Error("a tell marks the open sea")
		}
		if tl.X == dryX && tl.Y == dryY {
			foundDry = true
		}
	}
	if !foundDry {
		t.Error("the tell on living ground was not folded in")
	}
}

// TestFate_SaveLoadBranches: the chain round-trips through the DB,
// and sealing at a moment drops any previously sealed future.
func TestFate_SaveLoadBranches(t *testing.T) {
	conn := openMigratedDB(t)
	ctx := context.Background()
	f5 := Fate{Seed: 9, Kya: 5, Seats: []FateSeat{{X: 1, Y: 2, Tier: RegionOuthold, Name: "A", House: "HA"}}}
	f4 := Fate{Seed: 9, Kya: 4, Ruins: []FateRuin{{X: 3, Y: 4, Name: "B", Story: "fell"}}}
	for _, f := range []Fate{f5, f4} {
		if err := SaveFate(ctx, conn, f); err != nil {
			t.Fatalf("save fate kya=%d: %v", f.Kya, err)
		}
	}
	chain, err := LoadFateChain(ctx, conn, 9)
	if err != nil {
		t.Fatalf("load chain: %v", err)
	}
	if len(chain) != 2 || chain[0].Kya != 5 || chain[1].Kya != 4 {
		t.Fatalf("chain = %+v, want ages 5 then 4", chain)
	}
	if chain[0].Seats[0].House != "HA" || chain[1].Ruins[0].Story != "fell" {
		t.Error("the record lost detail in the round trip")
	}
	// Re-sealing age 5 abandons the old future at 4.
	if err := SaveFate(ctx, conn, Fate{Seed: 9, Kya: 5}); err != nil {
		t.Fatalf("re-seal: %v", err)
	}
	chain, err = LoadFateChain(ctx, conn, 9)
	if err != nil {
		t.Fatalf("reload chain: %v", err)
	}
	if len(chain) != 1 || chain[0].Kya != 5 {
		t.Fatalf("after re-sealing, chain = %+v, want only the new age 5", chain)
	}
	// Other seeds keep their own history.
	if other, _ := LoadFateChain(ctx, conn, 10); len(other) != 0 {
		t.Errorf("seed 10 inherited %d foreign ages", len(other))
	}
}
