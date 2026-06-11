package main

import (
	"testing"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

// TestBuildDangerMap_ActivityScales: the danger map must follow lair
// activity — double under a rampant dragon, gone under a dormant one,
// and exactly the static map when no activity source is given.
func TestBuildDangerMap_ActivityScales(t *testing.T) {
	features := []db.GetNamedFeaturesInBoundsRow{{X: 10, Y: 10, Kind: "den", Name: "Test Den"}}
	at := [2]int64{10, 11} // adjacent: cheb 1 → (12-1)*3 = 33

	static := buildDangerMap(features, nil)
	if static[at] != 33 {
		t.Fatalf("static danger at %v = %d, want 33", at, static[at])
	}
	rampant := buildDangerMap(features, func(x, y int64) float64 { return 2 })
	if rampant[at] != 66 {
		t.Errorf("rampant danger = %d, want 66", rampant[at])
	}
	dormant := buildDangerMap(features, func(x, y int64) float64 { return 0 })
	if len(dormant) != 0 {
		t.Errorf("dormant lair still projects %d danger cells", len(dormant))
	}
	rest := buildDangerMap(features, func(x, y int64) float64 { return 1 })
	if len(rest) != len(static) || rest[at] != static[at] {
		t.Error("activity 1 should reproduce the static map")
	}
}
