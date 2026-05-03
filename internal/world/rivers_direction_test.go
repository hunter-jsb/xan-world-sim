package world

import (
	"math/rand"
	"testing"
)

// TestRiversFlowDownhill checks that every river segment flows in a
// direction where the *bedrock* elevation actually decreases. Pit-fill
// raises cells in the working elevation field; if a basin cell got
// raised significantly above its neighbors, the flow algorithm would
// happily route water to a neighbor that's higher-than-self in the
// bedrock — looking, to a viewer who sees bedrock-shaded glyphs, like
// the river goes uphill. We want to know if this happens often.
func TestRiversFlowDownhill(t *testing.T) {
	for _, seed := range []int64{0, 42} {
		rng := rand.New(rand.NewSource(seed))
		bedrock := generateBedrock(rng)
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
		_, cells := traceRivers(bedrock, flowDir, accum, riverThreshold, riverMaxLenFor(0.0))

		var ns int // segments with neg dy = north (toward y=0)
		var ss int // segments with pos dy = south
		var ew int
		var sea int
		var uphill int
		var samelevel int
		for _, c := range cells {
			x, y := int(c.X), int(c.Y)
			d := flowDir[y][x]
			if d.dx == 0 && d.dy == 0 {
				sea++
				continue
			}
			nx, ny := x+d.dx, y+d.dy
			if nx < 0 || nx >= Width || ny < 0 || ny >= Height {
				sea++
				continue
			}
			selfE := bedrock[y][x].Elevation
			nE := bedrock[ny][nx].Elevation
			switch {
			case nE > selfE:
				uphill++
			case nE == selfE:
				samelevel++
			}
			switch {
			case d.dy < 0:
				ns++
			case d.dy > 0:
				ss++
			default:
				ew++
			}
		}
		t.Logf("seed=%d: %d river cells; flow-down dir: south=%d north=%d east/west=%d sink=%d; bedrock-uphill segments=%d (same=%d)",
			seed, len(cells), ss, ns, ew, sea, uphill, samelevel)
	}
}
