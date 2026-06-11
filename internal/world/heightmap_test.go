package world

import (
	"math/rand"
	"testing"
)

// TestElevationField_HasInternalVariation sanity-checks that within
// each zone, cells now have *different* elevations. If you accidentally
// turn off the noise (e.g., set all amplitudes to 0), this catches it.
func TestElevationField_HasInternalVariation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	bedrock := generateBedrock(rng)

	type stats struct {
		min, max, sum float64
		n             int
	}
	byZone := map[BedrockZone]*stats{}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			c := bedrock[y][x]
			s, ok := byZone[c.Zone]
			if !ok {
				s = &stats{min: c.Elevation, max: c.Elevation}
				byZone[c.Zone] = s
			}
			if c.Elevation < s.min {
				s.min = c.Elevation
			}
			if c.Elevation > s.max {
				s.max = c.Elevation
			}
			s.sum += c.Elevation
			s.n++
		}
	}

	zoneNames := map[BedrockZone]string{
		BZPlateau:       "plateau",
		BZMountain:      "mountain",
		BZCliff:         "cliff",
		BZFoothill:      "foothill",
		BZDoab:          "doab",
		BZCradle:        "cradle",
		BZBrineDeep:     "brine_deep",
		BZAgrariaShelf:  "shelf",
		BZAgrariaUpland: "upland",
		BZEastBasin:     "east_basin",
	}

	for z, s := range byZone {
		spread := s.max - s.min
		if s.n < 2 {
			continue // single-cell zone, no spread expected
		}
		if spread <= 0 {
			t.Errorf("zone %s has %d cells but zero spread (all elevation %.2fm)",
				zoneNames[z], s.n, s.min)
		}
		// Print for visibility.
		t.Logf("zone %-12s n=%-4d min=%-7.1f mean=%-7.1f max=%-7.1f spread=%.1fm",
			zoneNames[z], s.n, s.min, s.sum/float64(s.n), s.max, spread)
	}
}

// TestElevationField_CliffsPreserved verifies that smoothing didn't
// erode the big drops between zones. Adjacent cliff and cradle cells
// should still have a multi-thousand-meter drop.
func TestElevationField_CliffsPreserved(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	bedrock := generateBedrock(rng)

	// Find a cliff cell and check it's still much higher than any
	// adjacent cradle/foothill cell.
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if bedrock[y][x].Zone != BZCliff {
				continue
			}
			cliffElev := bedrock[y][x].Elevation
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
						continue
					}
					nz := bedrock[ny][nx].Zone
					if nz != BZCradle && nz != BZFoothill {
						continue
					}
					drop := cliffElev - bedrock[ny][nx].Elevation
					if drop < 1000 {
						t.Errorf("cliff at (%d,%d) elev=%.0fm has %s neighbor at (%d,%d) elev=%.0fm — drop only %.0fm, smoothing eroded the cliff",
							x, y, cliffElev,
							"neighbor", nx, ny, bedrock[ny][nx].Elevation, drop)
					}
				}
			}
		}
	}
}
