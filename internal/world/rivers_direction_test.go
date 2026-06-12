package world

import (
	"math/rand"
	"testing"
)

// TestRiversFlowDownhill bounds how far a river may climb in *bedrock*
// terms. Flow runs on the pit-filled elevation field, so segments that
// rise slightly in raw bedrock are expected — that's a basin the river
// flows through (rendered as a lake) — but the rise must stay within
// the cradle's noise amplitude (±50m, zoneAmplitude). A climb beyond
// that means flow directions were computed on the wrong field or
// pit-fill regressed: the river is visibly walking up a hillside.
func TestRiversFlowDownhill(t *testing.T) {
	// One noise amplitude of the zones rivers traverse (cradle 50m,
	// foothill 100m — cradle dominates; observed max climb is ~28m).
	const maxBedrockClimb = 50.0

	for _, seed := range []int64{0, 42, 7, 1234567890} {
		rng := rand.New(rand.NewSource(seed))
		bedrock, _ := generateBedrock(rng, seed, KyaNow)
		elev := make([][]float64, Height)
		for y := 0; y < Height; y++ {
			elev[y] = make([]float64, Width)
			for x := 0; x < Width; x++ {
				elev[y][x] = bedrock[y][x].Elevation
			}
		}
		fillPits(elev, bedrock)
		flowDir := computeFlowDirections(elev)
		accum := computeAccumulation(elev, bedrock, flowDir)
		_, cells := traceRivers(bedrock, flowDir, accum, riverThreshold(), riverMaxLenFor(0.0))

		if len(cells) == 0 {
			t.Errorf("seed=%d: fully-warm climate produced no river cells", seed)
			continue
		}

		var uphill, worstCount int
		var worstRise float64
		for _, c := range cells {
			x, y := int(c.X), int(c.Y)
			d := flowDir[y][x]
			if d.dx == 0 && d.dy == 0 {
				continue // local sink — chain ends here
			}
			nx, ny := x+d.dx, y+d.dy
			if !inBounds(nx, ny) {
				continue // flows off-map
			}
			rise := bedrock[ny][nx].Elevation - bedrock[y][x].Elevation
			if rise <= 0 {
				continue
			}
			uphill++
			if rise > worstRise {
				worstRise = rise
			}
			if rise > maxBedrockClimb {
				worstCount++
				if worstCount <= 3 { // don't flood the log
					t.Errorf("seed=%d: river at (%d,%d) climbs %.0fm of bedrock to (%d,%d) — max allowed %.0fm",
						seed, x, y, rise, nx, ny, maxBedrockClimb)
				}
			}
		}
		if worstCount > 3 {
			t.Errorf("seed=%d: ...and %d more over-limit climbs", seed, worstCount-3)
		}
		t.Logf("seed=%d: %d river cells, %d gentle uphill segments, worst climb %.1fm",
			seed, len(cells), uphill, worstRise)
	}
}
