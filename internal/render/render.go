package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

var glyphForKind = map[string]rune{
	"plateau":        '^',
	"mountain":       'A',
	"foothill":       'n',
	"cliff":          '|',
	"cradle":         '.',
	"doab":           '#',
	"sea_brine":      '%',
	"sea_eastern":    '~',
	"glacier":        '*',
	"agraria":        ';',
	"agraria_upland": '\'',
	"lake":           'o',
	"forest":         'T',
	"tundra":         '`',
	"unknown":        '?',
	"drowned":        '_',
}

// kindShading describes per-kind elevation-driven coloring. The
// renderer picks a tier (0..4) based on (elev - base) / amp, mapped
// from -1..1 to one of the 5 ANSI codes — lower elevations get
// darker shades, higher elevations get lighter ones. For zones
// where elevation isn't meaningful (e.g., glacier paints whatever
// is underneath at any temperature), shading still applies and
// gives subtle texture.
type kindShading struct {
	base, amp float64
	colors    [5]string // ANSI 256 codes, low → high
	bold      bool      // applied to all tiers
}

var shadingByKind = map[string]kindShading{
	// gray ramp — plateau: weathered stone → sun-bleached/snow-touched top
	"plateau": {base: 1500, amp: 200, colors: [5]string{"248", "250", "253", "255", "231"}},
	// stone gray ramp — mountains: dark base to light peaks
	"mountain": {base: 3000, amp: 500, colors: [5]string{"238", "241", "244", "248", "252"}},
	// stone, narrower band — cliffs are tall but constrained
	"cliff": {base: 2500, amp: 200, colors: [5]string{"240", "243", "246", "249", "252"}, bold: true},
	// olive/khaki ramp — foothills: dark grass-soil → drier/lighter highs
	"foothill": {base: 500, amp: 100, colors: [5]string{"100", "107", "143", "179", "186"}},
	// brown ramp — doab: dark earth → weathered rock
	"doab": {base: 2000, amp: 200, colors: [5]string{"94", "130", "137", "180", "187"}},
	// green ramp — cradle: shadowed forest → bright meadow
	"cradle": {base: 100, amp: 50, colors: [5]string{"22", "28", "34", "70", "107"}},
	// deep blue ramp — Brine: abyssal → shoaling
	"sea_brine": {base: -800, amp: 100, colors: [5]string{"17", "18", "19", "20", "27"}},
	// cyan ramp — Eastern Sea: deeper basin → shoals
	"sea_eastern": {base: -150, amp: 50, colors: [5]string{"24", "31", "38", "45", "51"}},
	// icy ramp — glacier
	"glacier": {base: 0, amp: 1500, colors: [5]string{"152", "153", "159", "195", "231"}, bold: true},
	// muted yellow-tan — Agraria coast (lower)
	"agraria": {base: -80, amp: 15, colors: [5]string{"137", "143", "144", "179", "180"}},
	// brighter tan — Agraria upland (higher)
	"agraria_upland": {base: -40, amp: 15, colors: [5]string{"143", "179", "180", "215", "222"}},
	// lake — pale blue, distinct from rivers (bright bold cyan) so
	// you can see lakes adjacent to rivers
	"lake": {base: 100, amp: 50, colors: [5]string{"31", "38", "45", "81", "117"}, bold: true},
	// forest — darker green than cradle's grassland-y default; bumpy
	// canopy reads against `T` glyph
	"forest": {base: 100, amp: 50, colors: [5]string{"22", "28", "29", "65", "71"}},
	// tundra — pale gray-green; cold and sparse
	"tundra": {base: 100, amp: 50, colors: [5]string{"101", "108", "144", "145", "151"}},
	"unknown":        {base: 0, amp: 1, colors: [5]string{"99", "99", "99", "99", "99"}},
	"drowned":        {base: -800, amp: 100, colors: [5]string{"60", "60", "60", "60", "60"}},
}

// styleFor returns a lipgloss Style for a cell of the given kind at
// the given elevation. Tier is computed from elev's deviation from
// the kind's base, normalized by its amp. Cached per (kind, tier).
func styleFor(kind string, elev float64) lipgloss.Style {
	s, ok := shadingByKind[kind]
	if !ok {
		return lipgloss.NewStyle()
	}
	tier := elevTier(elev, s.base, s.amp)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(s.colors[tier]))
	if s.bold {
		style = style.Bold(true)
	}
	return style
}

func elevTier(elev, base, amp float64) int {
	if amp <= 0 {
		return 2
	}
	norm := (elev - base) / amp // -1..1ish
	if norm < -0.6 {
		return 0
	}
	if norm < -0.2 {
		return 1
	}
	if norm < 0.2 {
		return 2
	}
	if norm < 0.6 {
		return 3
	}
	return 4
}

var (
	emptyStyle = lipgloss.NewStyle()
	riverStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

const (
	emptyGlyph = ' '
	riverGlyph = ','
)

func Grid(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, minX, minY, maxX, maxY int64) string {
	width := int(maxX - minX + 1)
	height := int(maxY - minY + 1)
	if width <= 0 || height <= 0 {
		return ""
	}
	grid := make([][]string, height)
	for i := range grid {
		grid[i] = make([]string, width)
		for j := range grid[i] {
			grid[i][j] = " "
		}
	}
	for _, c := range cells {
		gy, gx := int(c.Y-minY), int(c.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		g, ok := glyphForKind[c.Kind]
		if !ok {
			g = '?'
		}
		grid[gy][gx] = styleFor(c.Kind, c.Elevation).Render(string(g))
	}
	for _, r := range rivers {
		gy, gx := int(r.Y-minY), int(r.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		grid[gy][gx] = riverStyle.Render(string(riverGlyph))
	}

	var b strings.Builder
	for i, row := range grid {
		b.WriteString(strings.Join(row, ""))
		if i < len(grid)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func Title(s string) string {
	return titleStyle.Render(s)
}

func Legend() string {
	item := func(kind string, label string) string {
		g := glyphForKind[kind]
		s := shadingByKind[kind]
		// Use mid tier for legend swatch.
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(s.colors[2]))
		if s.bold {
			st = st.Bold(true)
		}
		return st.Render(string(g)) + dimStyle.Render(" "+label)
	}
	row1 := strings.Join([]string{
		item("plateau", "plateau"),
		item("mountain", "mountain"),
		item("foothill", "foothill"),
		item("cliff", "cliff"),
	}, "   ")
	row2 := strings.Join([]string{
		item("cradle", "cradle"),
		item("sea_brine", "brine"),
		item("sea_eastern", "eastern sea"),
		riverStyle.Render(",") + dimStyle.Render(" river"),
	}, "   ")
	row3 := strings.Join([]string{
		item("glacier", "glacier"),
		item("agraria", "agraria coast"),
		item("agraria_upland", "agraria upland"),
		item("lake", "lake"),
	}, "   ")
	row4 := strings.Join([]string{
		item("forest", "forest"),
		item("tundra", "tundra"),
	}, "   ")
	return row1 + "\n" + row2 + "\n" + row3 + "\n" + row4
}
