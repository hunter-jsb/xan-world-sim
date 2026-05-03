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
		c := c
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
		{KyaNow, 0, "1aab33658aff6f3b1d06ffe2ec6cc6eba0b55e54ee104597895670bad775f29b"},
		{KyaNow, 42, "80810cdd2210d2683d12557dfd6e76ebe50407ba3e6a3aaa32cf1320ad8b8de0"},
		{KyaOldWorld, 0, "04cb6bbcaa54781ea2587529972919803669baef2bb8a2a66df65f5831e0d7af"},
		{KyaOldWorld, 42, "7dde3191c7d91e428778e12a72a7f9168dc1802a5ee9eeb14bf75b70923c410a"},
		{100, 42, "ddc7624e5694ff3bc1dbb36c4ebcf45c4df52c503cda27410cbf65dc951f999a"}, // mid-cycle
	}
	for _, c := range cases {
		c := c
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
		fmt.Fprintf(&b, "RI(%d,%s)|", ri.ID, ri.Name)
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
		fmt.Fprintf(&b, "S(%d,%d,%d,%s)|", s.X, s.Y, s.Tier, s.Name)
	}

	lakes := make([]LakeInfo, len(w.Lakes))
	copy(lakes, w.Lakes)
	sort.Slice(lakes, func(i, j int) bool { return lakes[i].ID < lakes[j].ID })
	for _, l := range lakes {
		fmt.Fprintf(&b, "L(%d,%d,%d,%s)|", l.ID, l.X, l.Y, l.Name)
	}

	passes := make([]PassInfo, len(w.Passes))
	copy(passes, w.Passes)
	sort.Slice(passes, func(i, j int) bool { return passes[i].ID < passes[j].ID })
	for _, p := range passes {
		fmt.Fprintf(&b, "P(%d,%d,%d,%s)|", p.ID, p.X, p.Y, p.Name)
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
