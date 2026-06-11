package world

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// TestGenerate_Deterministic catches the nondeterminism class of bug:
// same seed + era should produce byte-identical output across two
// consecutive calls in the same process. If a future change introduces
// map iteration into an RNG-consuming loop, or accidentally calls
// rand.Intn (global) instead of rng.Intn, this test fails immediately.
func TestGenerate_Deterministic(t *testing.T) {
	cases := []struct {
		kya  int
		seed int64
	}{
		{KyaNow, 0},
		{KyaNow, 1},
		{KyaNow, 42},
		{KyaNow, 1234567890},
		{KyaOldWorld, 0},
		{KyaOldWorld, 42},
		{100, 42}, // mid-cycle
		{50, 42},  // post-LGM warming
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("kya=%d/seed=%d", c.kya, c.seed), func(t *testing.T) {
			a := hashWorld(Generate(c.seed, c.kya))
			b := hashWorld(Generate(c.seed, c.kya))
			if a != b {
				t.Fatalf("two calls produced different worlds:\n  a=%s\n  b=%s", a, b)
			}
		})
	}
}

// TestGenerate_Snapshot pins the world output for known seed+era pairs.
// If you intentionally change worldgen, update the expected hash in
// this file in the same commit. If the hash changes unintentionally,
// this test screams.
func TestGenerate_Snapshot(t *testing.T) {
	cases := []struct {
		kya      int
		seed     int64
		expected string
	}{
		{KyaNow, 0, "8df258687346b1d8b2484f2dcc4558f11963050ec4be070b6844eafb09fca966"},
		{KyaNow, 42, "0e8bf7eea75a935b9635fc8ddf20bfd3b58bdf6755d8711ed0b9ad074b8a2c4e"},
		{KyaOldWorld, 0, "ef89aeb02aecfe9a2d2149fe4feb80dda6d624854f650ddce77078242be87643"},
		{KyaOldWorld, 42, "858cb945fc3bebe0eef83cf7ec5920d91e12fb89f3103b4b58edfa1c2e0d08be"},
		{100, 42, "1302fc7b0bd7645ff9066d06e02849f9288183b82d46f2091cf482835df24aeb"}, // mid-cycle
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("kya=%d/seed=%d", c.kya, c.seed), func(t *testing.T) {
			got := hashWorld(Generate(c.seed, c.kya))
			if strings.HasPrefix(c.expected, "REPLACE_ME") {
				t.Logf("snapshot hash (paste into test): %s", got)
				t.Fatalf("expected value not yet pinned for kya=%d/seed=%d", c.kya, c.seed)
			}
			if got != c.expected {
				t.Fatalf("snapshot drift\n  kya:  %d\n  seed: %d\n  got:  %s\n  want: %s\n\nIf this drift is intentional, update the expected value in generate_test.go.",
					c.kya, c.seed, got, c.expected)
			}
		})
	}
}

// hashWorld produces a stable sha256 hex of every meaningful field on
// a World. Sorts cells before hashing so the hash is invariant under
// emission-order changes (we'd be detecting world content drift, not
// "we re-ordered some loops" drift).
func hashWorld(w World) string {
	var b strings.Builder
	fmt.Fprintf(&b, "seed=%d|kya=%d|era=%s|", w.Seed, w.Kya, w.Era)
	fmt.Fprintf(&b, "lat=%g..%g|", w.LatTop, w.LatBottom)
	fmt.Fprintf(&b, "orb=%g,%g,%g|",
		w.Orbital.Obliquity, w.Orbital.Eccentricity, w.Orbital.Precession)
	fmt.Fprintf(&b, "clim=%g,%g,%g|",
		w.Climate.SeaLevelDelta, w.Climate.GlacialIndex, w.Climate.GlobalMeanTempDelta)

	regs := make([]RegionCell, len(w.Regions))
	copy(regs, w.Regions)
	sort.Slice(regs, func(i, j int) bool {
		if regs[i].Y != regs[j].Y {
			return regs[i].Y < regs[j].Y
		}
		if regs[i].X != regs[j].X {
			return regs[i].X < regs[j].X
		}
		return regs[i].RegionID < regs[j].RegionID
	})
	for _, r := range regs {
		fmt.Fprintf(&b, "R(%d,%d,%d,%.1f)|", r.RegionID, r.X, r.Y, r.Elevation)
	}

	infos := make([]River, len(w.RiverInfo))
	copy(infos, w.RiverInfo)
	sort.Slice(infos, func(i, j int) bool { return infos[i].ID < infos[j].ID })
	for _, ri := range infos {
		fmt.Fprintf(&b, "RI(%d,%s,%d)|", ri.ID, ri.Name, ri.Drainage)
	}

	seats := make([]NamedSeat, len(w.Seats))
	copy(seats, w.Seats)
	sort.Slice(seats, func(i, j int) bool {
		if seats[i].Y != seats[j].Y {
			return seats[i].Y < seats[j].Y
		}
		return seats[i].X < seats[j].X
	})
	for _, s := range seats {
		fmt.Fprintf(&b, "S(%d,%d,%d,%s,%.1f,%d,%.3f)|",
			s.X, s.Y, s.Tier, s.Name, s.Pressure, s.RealmID, s.Allegiance)
	}

	realms := make([]Realm, len(w.Realms))
	copy(realms, w.Realms)
	sort.Slice(realms, func(i, j int) bool { return realms[i].ID < realms[j].ID })
	for _, r := range realms {
		fmt.Fprintf(&b, "RM(%d,%s,%t,%d,%d)|", r.ID, r.Name, r.IsCrown, r.SeatX, r.SeatY)
	}

	terr := make([]TerritoryCell, len(w.Territory))
	copy(terr, w.Territory)
	sort.Slice(terr, func(i, j int) bool {
		if terr[i].Y != terr[j].Y {
			return terr[i].Y < terr[j].Y
		}
		return terr[i].X < terr[j].X
	})
	for _, tc := range terr {
		fmt.Fprintf(&b, "T(%d,%d,%d)|", tc.X, tc.Y, tc.RealmID)
	}

	lakes := make([]LakeInfo, len(w.Lakes))
	copy(lakes, w.Lakes)
	sort.Slice(lakes, func(i, j int) bool { return lakes[i].ID < lakes[j].ID })
	for _, l := range lakes {
		fmt.Fprintf(&b, "L(%d,%d,%d,%s,%.1f,%.1f)|", l.ID, l.X, l.Y, l.Name, l.SurfaceElev, l.MaxDepth)
	}

	passes := make([]PassInfo, len(w.Passes))
	copy(passes, w.Passes)
	sort.Slice(passes, func(i, j int) bool { return passes[i].ID < passes[j].ID })
	for _, p := range passes {
		fmt.Fprintf(&b, "P(%d,%d,%d,%s)|", p.ID, p.X, p.Y, p.Name)
	}

	roads := make([]Road, len(w.Roads))
	copy(roads, w.Roads)
	sort.Slice(roads, func(i, j int) bool { return roads[i].ID < roads[j].ID })
	for _, r := range roads {
		fmt.Fprintf(&b, "RD(%d,%d,%d→%d,%d)|", r.ID, r.FromX, r.FromY, r.ToX, r.ToY)
	}
	rcs := make([]RoadCell, len(w.RoadCells))
	copy(rcs, w.RoadCells)
	sort.Slice(rcs, func(i, j int) bool {
		if rcs[i].RoadID != rcs[j].RoadID {
			return rcs[i].RoadID < rcs[j].RoadID
		}
		return rcs[i].Ord < rcs[j].Ord
	})
	for _, rc := range rcs {
		fmt.Fprintf(&b, "RDC(%d,%d,%d,%d)|", rc.RoadID, rc.X, rc.Y, rc.Ord)
	}

	dens := make([]DenInfo, len(w.Dens))
	copy(dens, w.Dens)
	sort.Slice(dens, func(i, j int) bool { return dens[i].ID < dens[j].ID })
	for _, d := range dens {
		fmt.Fprintf(&b, "D(%d,%d,%d,%.1f,%s)|", d.ID, d.X, d.Y, d.Elevation, d.Name)
	}

	nests := make([]NestInfo, len(w.Nests))
	copy(nests, w.Nests)
	sort.Slice(nests, func(i, j int) bool { return nests[i].ID < nests[j].ID })
	for _, n := range nests {
		fmt.Fprintf(&b, "N(%d,%d,%d,%.1f,%s)|", n.ID, n.X, n.Y, n.Elevation, n.Name)
	}

	rooks := make([]RookeryInfo, len(w.Rookeries))
	copy(rooks, w.Rookeries)
	sort.Slice(rooks, func(i, j int) bool { return rooks[i].ID < rooks[j].ID })
	for _, r := range rooks {
		fmt.Fprintf(&b, "W(%d,%d,%d,%.1f,%s)|", r.ID, r.X, r.Y, r.Elevation, r.Name)
	}

	rivs := make([]RiverCell, len(w.Rivers))
	copy(rivs, w.Rivers)
	sort.Slice(rivs, func(i, j int) bool {
		if rivs[i].RiverID != rivs[j].RiverID {
			return rivs[i].RiverID < rivs[j].RiverID
		}
		return rivs[i].Ord < rivs[j].Ord
	})
	for _, r := range rivs {
		fmt.Fprintf(&b, "V(%d,%d,%d,%d)|", r.RiverID, r.X, r.Y, r.Ord)
	}

	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h)
}
