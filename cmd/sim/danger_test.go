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

	static, src := buildDangerMap(features, nil, nil)
	if static[at] != 33 {
		t.Fatalf("static danger at %v = %d, want 33", at, static[at])
	}
	if src[at] != "den" {
		t.Errorf("danger source at %v = %q, want den", at, src[at])
	}
	rampant, _ := buildDangerMap(features, func(x, y int64) float64 { return 2 }, nil)
	if rampant[at] != 66 {
		t.Errorf("rampant danger = %d, want 66", rampant[at])
	}
	dormant, _ := buildDangerMap(features, func(x, y int64) float64 { return 0 }, nil)
	if len(dormant) != 0 {
		t.Errorf("dormant lair still projects %d danger cells", len(dormant))
	}
	rest, _ := buildDangerMap(features, func(x, y int64) float64 { return 1 }, nil)
	if len(rest) != len(static) || rest[at] != static[at] {
		t.Error("activity 1 should reproduce the static map")
	}
}

// TestBuildDangerMap_VolcanoHeat: a vent radiates through the same
// map — full flame fresh after an eruption, scaled by the persisted
// age when static, cold once it has cooled past volcanoCoolKa, and
// live heat outranks the persisted age in a slice.
func TestBuildDangerMap_VolcanoHeat(t *testing.T) {
	fresh := []db.GetNamedFeaturesInBoundsRow{{X: 10, Y: 10, Kind: "volcano", Name: "Brimon", Meta: 0}}
	at := [2]int64{10, 11} // cheb 1 → (8-1)*3 = 21 at heat 1

	d, src := buildDangerMap(fresh, nil, nil)
	if d[at] != 21 {
		t.Fatalf("fresh vent danger = %d, want 21", d[at])
	}
	if src[at] != "volcano" {
		t.Errorf("danger source = %q, want volcano", src[at])
	}

	half := []db.GetNamedFeaturesInBoundsRow{{X: 10, Y: 10, Kind: "volcano", Name: "Brimon", Meta: 25}}
	d, _ = buildDangerMap(half, nil, nil)
	if want := 11; d[at] != want { // heat 0.5 → 10.5 → rounds to 11
		t.Errorf("half-cooled vent danger = %d, want %d", d[at], want)
	}

	cold := []db.GetNamedFeaturesInBoundsRow{{X: 10, Y: 10, Kind: "volcano", Name: "Brimon", Meta: 200}}
	d, _ = buildDangerMap(cold, nil, nil)
	if len(d) != 0 {
		t.Errorf("cold vent still projects %d danger cells", len(d))
	}

	// Live slice heat wins over the persisted age.
	d, _ = buildDangerMap(cold, nil, func(x, y int64) float64 { return 1 })
	if d[at] != 21 {
		t.Errorf("live-erupted vent danger = %d, want 21", d[at])
	}
}
