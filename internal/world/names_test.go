package world

import (
	"testing"
	"unicode"
)

func TestGenerateName_Deterministic(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, -7, 1 << 40} {
		a, b := generateName(seed), generateName(seed)
		if a != b {
			t.Errorf("generateName(%d) not deterministic: %q vs %q", seed, a, b)
		}
		if a == "" {
			t.Errorf("generateName(%d) returned empty name", seed)
		}
		runes := []rune(a)
		if !unicode.IsUpper(runes[0]) {
			t.Errorf("generateName(%d) = %q, want leading capital", seed, a)
		}
		for _, r := range runes[1:] {
			if !unicode.IsLower(r) {
				t.Errorf("generateName(%d) = %q, want lowercase after first rune", seed, a)
			}
		}
	}
}

func TestNameSeedForCell_SpreadsAdjacentCells(t *testing.T) {
	// Adjacent cells on the same world must get distinct name seeds —
	// the prime mixing exists so neighboring features don't share names.
	const worldSeed = 42
	seen := make(map[int64][2]int64)
	for y := int64(0); y < 30; y++ {
		for x := int64(0); x < 80; x++ {
			s := nameSeedForCell(worldSeed, x, y)
			if prev, dup := seen[s]; dup {
				t.Fatalf("name seed collision: (%d,%d) and (%d,%d) both → %d",
					prev[0], prev[1], x, y, s)
			}
			seen[s] = [2]int64{x, y}
		}
	}
}

func TestGenerateName_VarietyAcrossSeeds(t *testing.T) {
	// Not a uniqueness guarantee (the phoneme space is finite), but a
	// sanity bound: 200 distinct seeds should not funnel into a handful
	// of names.
	names := make(map[string]bool)
	for i := int64(0); i < 200; i++ {
		names[generateName(nameSeedForCell(1, i%20, i/20))] = true
	}
	if len(names) < 100 {
		t.Errorf("200 seeds produced only %d distinct names — distribution looks broken", len(names))
	}
}
