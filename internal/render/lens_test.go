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

// TestRockColor_LithologyTotal: every rock type the world can emit
// reads distinctly, fresh lava glows, and young drift bolds.
func TestRockColor_LithologyTotal(t *testing.T) {
	seen := map[string]bool{}
	for rock := int64(1); rock <= 7; rock++ {
		c, _ := RockColor(rock, 100)
		if c == "" {
			t.Errorf("rock %d has no color", rock)
		}
		seen[c] = true
	}
	if len(seen) != 7 {
		t.Errorf("only %d distinct colors across 7 rock types", len(seen))
	}
	if c, bold := RockColor(7, 5); c != "196" || !bold {
		t.Errorf("fresh lava = (%s,%v), want glowing 196 bold", c, bold)
	}
	if c, bold := RockColor(7, 100); c == "196" || bold {
		t.Errorf("old basalt = (%s,%v), want weathered and quiet", c, bold)
	}
	if _, bold := RockColor(5, 10); !bold {
		t.Error("till the ice just left should read bold")
	}
	if _, bold := RockColor(5, 300); bold {
		t.Error("ancient till should read quiet")
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
	rockAt := func(x, y int64) (int64, int64) { return 1 + (x+y)%7, 100 }
	builders := map[string]*GridBuf{
		"climate": BuildClimateGridBuf(cells, nil, nil, 0, 0, 1, 1, tempAt),
		"geo":     BuildGeoGridBuf(cells, nil, nil, 0, 0, 1, 1, rockAt),
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

// TestDrainageAndDangerBands: both ramps are total and distinct
// across their working spans.
func TestDrainageAndDangerBands(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range []int64{1, 8, 32, 128, 512, 2048} {
		c, _ := DrainageColor("cradle", d)
		if c == "" {
			t.Errorf("no drainage band for %d", d)
		}
		seen[c] = true
	}
	if len(seen) != 6 {
		t.Errorf("drainage ramp has %d distinct bands over 6 spans", len(seen))
	}
	if c, _ := DrainageColor("sea_brine", 0); c != "17" {
		t.Errorf("open water should read flat deep blue, got %s", c)
	}
	seen = map[string]bool{}
	for _, d := range []int{0, 3, 8, 15, 25, 40} {
		c, _ := DangerColor("cradle", d)
		if c == "" {
			t.Errorf("no danger band for %d", d)
		}
		seen[c] = true
	}
	if len(seen) != 6 {
		t.Errorf("danger ramp has %d distinct bands over 6 spans", len(seen))
	}
}
