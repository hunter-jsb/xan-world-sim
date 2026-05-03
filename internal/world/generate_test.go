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
		{KyaNow, 0, "aec1fcfe2cd153c275321e5a3d76729f953f8d820f19ef3218e2a4341d75e1a7"},
		{KyaNow, 42, "da0bd8bc79ca1501d5e4f1a73173d97f5e5adc35df8990848e442a188a873bd5"},
		{KyaOldWorld, 0, "c23be053f7424050cadd83c8b2d81aa6bb3cf23c0c66387d8d18478fd893e4ea"},
		{KyaOldWorld, 42, "d5a82461a1d6b50b95f8bd7782195167871f11c72c77a7e93c10c2c4ec3d29e2"},
		{100, 42, "66f9346886bf83270c7935755ba63127235285237741f14e65ed3e491d4fd149"}, // mid-cycle
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
