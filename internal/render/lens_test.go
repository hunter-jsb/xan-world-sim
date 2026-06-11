package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// TestEcoClassColor_CoversEveryKind keeps the ecological lens total:
// every region kind the world can emit must land in a life zone.
func TestEcoClassColor_CoversEveryKind(t *testing.T) {
	for id := int64(1); ; id++ {
		kind := world.RegionKind(id)
		if kind == "" {
			break
		}
		if c, _ := ecoClassColor(kind); c == "" {
			t.Errorf("kind %q has no ecological class color", kind)
		}
	}
}

// TestClimateBand_Ordering: the ramp must be total and change across
// the world's temperature span — frozen, freezing edge, temperate,
// hot must all read differently.
func TestClimateBand_Ordering(t *testing.T) {
	temps := []float64{-20, -5, 0, 5, 11, 17, 23, 30}
	seen := map[string]bool{}
	for _, temp := range temps {
		c, _ := climateBand(temp)
		if c == "" {
			t.Errorf("no band for %g°C", temp)
		}
		seen[c] = true
	}
	if len(seen) != len(temps) {
		t.Errorf("only %d distinct bands across %d sample temps", len(seen), len(temps))
	}
}

// TestGeoBand_SpansBedrock: distinct bands from the abyss to peaks.
func TestGeoBand_SpansBedrock(t *testing.T) {
	elevs := []float64{-3000, -1000, -100, 100, 500, 1000, 2000, 3000, 4000}
	seen := map[string]bool{}
	for _, e := range elevs {
		c, _ := geoBand(e)
		if c == "" {
			t.Errorf("no band for %gm", e)
		}
		seen[c] = true
	}
	if len(seen) != len(elevs) {
		t.Errorf("only %d distinct bands across %d sample elevations", len(seen), len(elevs))
	}
}

// TestLensBuilders_RenderCleanly: each lens renders the same grid
// shape with full-width rows and untouched glyphs.
func TestLensBuilders_RenderCleanly(t *testing.T) {
	cells := []db.GetCellsInBoundsRow{
		{X: 0, Y: 0, Kind: "cradle", Elevation: 100},
		{X: 1, Y: 0, Kind: "mountain", Elevation: 3000},
		{X: 0, Y: 1, Kind: "sea_brine", Elevation: -800},
		{X: 1, Y: 1, Kind: "forest", Elevation: 120},
	}
	tempAt := func(x, y int64, elev float64) float64 {
		return world.Temperature(world.Latitude(int(y), world.DefaultLatTop, world.DefaultLatBottom),
			elev, world.ClimateAt(0))
	}
	builders := map[string]*GridBuf{
		"climate": BuildClimateGridBuf(cells, nil, nil, 0, 0, 1, 1, tempAt),
		"geo":     BuildGeoGridBuf(cells, nil, nil, 0, 0, 1, 1),
		"eco":     BuildEcoGridBuf(cells, nil, nil, 0, 0, 1, 1),
	}
	for name, gb := range builders {
		out := gb.Render(-1, -1, nil)
		rows := strings.Split(stripANSI(out), "\n")
		if len(rows) != 2 {
			t.Fatalf("%s lens: %d rows, want 2", name, len(rows))
		}
		if rows[0] != ".A" || rows[1] != "%T" {
			t.Errorf("%s lens changed glyphs: %q %q — lenses recolor, never redraw", name, rows[0], rows[1])
		}
		for i, r := range strings.Split(out, "\n") {
			if w := lipgloss.Width(r); w != 2 {
				t.Errorf("%s lens row %d width %d, want 2", name, i, w)
			}
		}
	}
}
