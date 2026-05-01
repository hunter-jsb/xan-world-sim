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
		era  Era
		seed int64
	}{
		{EraNow, 0},
		{EraNow, 1},
		{EraNow, 42},
		{EraNow, 1234567890},
		{EraOldWorld, 0},
		{EraOldWorld, 42},
	}
	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%s/seed=%d", c.era, c.seed), func(t *testing.T) {
			a := hashWorld(Generate(c.seed, c.era))
			b := hashWorld(Generate(c.seed, c.era))
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
		era      Era
		seed     int64
		expected string
	}{
		{EraNow, 0, "a6d55392620ad90aca6424d8990852af5893527eecf8aa78aa5a8791da38c339"},
		{EraNow, 42, "9bb20192b7fe838f8523b4fed6184621c2174eb61e095dde7200449f76b48444"},
		{EraOldWorld, 0, "72497bd4a28f2448454a9c2af6945f811a49618fac5759e1da6b7e199e9ce496"},
		{EraOldWorld, 42, "9d4da141f95da08f95258fa05bde93238fbdb5ac9aebd9f2d7ef5787debb6d8a"},
	}
	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%s/seed=%d", c.era, c.seed), func(t *testing.T) {
			got := hashWorld(Generate(c.seed, c.era))
			if strings.HasPrefix(c.expected, "REPLACE_ME") {
				t.Logf("snapshot hash (paste into test): %s", got)
				t.Fatalf("expected value not yet pinned for %s/seed=%d", c.era, c.seed)
			}
			if got != c.expected {
				t.Fatalf("snapshot drift\n  era:  %s\n  seed: %d\n  got:  %s\n  want: %s\n\nIf this drift is intentional, update the expected value in generate_test.go.",
					c.era, c.seed, got, c.expected)
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
	fmt.Fprintf(&b, "seed=%d|era=%s|", w.Seed, w.Era)
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
		fmt.Fprintf(&b, "R(%d,%d,%d)|", r.RegionID, r.X, r.Y)
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
