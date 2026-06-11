package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

// DirectionalGlyph returns a glyph for a step in direction (dx, dy).
// Used for both river flow and expedition path rendering.
func DirectionalGlyph(dx, dy int) rune {
	switch {
	case dx > 0 && dy == 0:
		return '>'
	case dx < 0 && dy == 0:
		return '<'
	case dx == 0 && dy > 0:
		return 'v'
	case dx > 0 && dy > 0:
		return '\\'
	case dx < 0 && dy > 0:
		return '/'
	case dx > 0 && dy < 0:
		return '/'
	case dx < 0 && dy < 0:
		return '\\'
	default:
		return ','
	}
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

func shade(base, amp float64, colors [5]string) kindShading {
	return kindShading{base: base, amp: amp, colors: colors}
}

func boldShade(base, amp float64, colors [5]string) kindShading {
	return kindShading{base: base, amp: amp, colors: colors, bold: true}
}

// kindSpec is everything the renderer knows about one region kind:
// map glyph, elevation shading, and the display labels the info panel
// uses. Adding a region kind means adding exactly one entry here (plus
// the world-package RegionID mapping and the migration).
type kindSpec struct {
	glyph   rune
	shading kindShading

	// label is the plain-terrain name shown in the info panel.
	label string

	// tierLabel is set for seat-tier kinds (seat/march/headwater/
	// outhold/reach) — the lore name of the seat tier.
	tierLabel string

	// featureLabel is set for named-feature kinds (den/nest/rookery/
	// lake/pass) — the lead-in shown next to the feature's procgen name.
	featureLabel string
}

var kinds = map[string]kindSpec{
	// gray ramp — plateau: weathered stone → sun-bleached/snow-touched top
	"plateau": {glyph: '^', label: "plateau",
		shading: shade(1500, 200, [5]string{"248", "250", "253", "255", "231"})},
	// stone gray ramp — mountains: dark base to light peaks
	"mountain": {glyph: 'A', label: "mountain",
		shading: shade(3000, 500, [5]string{"238", "241", "244", "248", "252"})},
	// stone, narrower band — cliffs are tall but constrained
	"cliff": {glyph: '|', label: "cliff",
		shading: boldShade(2500, 200, [5]string{"240", "243", "246", "249", "252"})},
	// olive/khaki ramp — foothills: dark grass-soil → drier/lighter highs
	"foothill": {glyph: 'n', label: "foothill",
		shading: shade(500, 100, [5]string{"100", "107", "143", "179", "186"})},
	// brown ramp — doab: dark earth → weathered rock
	"doab": {glyph: '#', label: "doab",
		shading: shade(2000, 200, [5]string{"94", "130", "137", "180", "187"})},
	// green ramp — cradle: shadowed forest → bright meadow
	"cradle": {glyph: '.', label: "cradle",
		shading: shade(100, 50, [5]string{"22", "28", "34", "70", "107"})},
	// deep blue ramp — Brine: abyssal → shoaling
	"sea_brine": {glyph: '%', label: "Brine",
		shading: shade(-800, 100, [5]string{"17", "18", "19", "20", "27"})},
	// cyan ramp — Eastern Sea: deeper basin → shoals
	"sea_eastern": {glyph: '~', label: "Eastern Sea",
		shading: shade(-150, 50, [5]string{"24", "31", "38", "45", "51"})},
	// icy ramp — glacier
	"glacier": {glyph: '*', label: "glacier",
		shading: boldShade(0, 1500, [5]string{"152", "153", "159", "195", "231"})},
	// muted yellow-tan — Agraria coast (lower)
	"agraria": {glyph: ';', label: "Agraria coast",
		shading: shade(-80, 15, [5]string{"137", "143", "144", "179", "180"})},
	// brighter tan — Agraria upland (higher)
	"agraria_upland": {glyph: '\'', label: "Agraria upland",
		shading: shade(-40, 15, [5]string{"143", "179", "180", "215", "222"})},
	// lake — pale blue, distinct from rivers (bright bold cyan) so
	// you can see lakes adjacent to rivers
	"lake": {glyph: 'o', label: "lake", featureLabel: "lake",
		shading: boldShade(100, 50, [5]string{"31", "38", "45", "81", "117"})},
	// forest — darker green than cradle's grassland-y default; bumpy
	// canopy reads against `T` glyph
	"forest": {glyph: 'T', label: "forest",
		shading: shade(100, 50, [5]string{"22", "28", "29", "65", "71"})},
	// tundra — pale gray-green; cold and sparse
	"tundra": {glyph: '`', label: "tundra",
		shading: shade(100, 50, [5]string{"101", "108", "144", "145", "151"})},
	// marsh — muddy yellow-green, water-reflecting; sits between
	// forest and the water it borders
	"marsh": {glyph: '=', label: "marsh",
		shading: shade(100, 50, [5]string{"58", "100", "107", "143", "108"})},
	// seat — bold gold, civilization marker on a river chain
	"seat": {glyph: 'H', label: "seat", tierLabel: "Tributary",
		shading: boldShade(100, 50, [5]string{"178", "214", "220", "226", "227"})},
	// march — slate steel, the wall against the mountain wilds; cooler
	// and more austere than the gold of a salmon-lord's hall
	"march": {glyph: 'M', label: "march", tierLabel: "March",
		shading: boldShade(500, 200, [5]string{"60", "67", "74", "110", "117"})},
	// headwater — pale silver-cyan, sacred ice-melt source; brighter
	// at high elevation since the headwaters of major rivers sit high
	"headwater": {glyph: 'Y', label: "headwater", tierLabel: "Headwater Hold",
		shading: boldShade(800, 300, [5]string{"81", "117", "153", "159", "195"})},
	// outhold — muted ochre, the off-grid catch-all; less imposing than
	// the other seat tiers, suited to ranchers/prospectors/fugitives
	"outhold": {glyph: 'x', label: "outhold", tierLabel: "Outhold",
		shading: shade(200, 200, [5]string{"94", "130", "137", "172", "179"})},
	// reach — bold magenta-violet, the frontier-explorer hold; visually
	// distinct from earthier tiers since Reaches are exceptional & rare
	"reach": {glyph: 'R', label: "reach", tierLabel: "Reach",
		shading: boldShade(200, 200, [5]string{"54", "91", "127", "163", "207"})},
	// pass — yellow-stone, a saddle through the mountain ridge; bright
	// against the surrounding gray mountains so passes stand out
	"pass": {glyph: 'V', label: "mountain pass", featureLabel: "mountain pass",
		shading: boldShade(2500, 300, [5]string{"178", "214", "220", "229", "230"})},
	// den — blood-crimson, the dragon's lair; deeper red at higher
	// elevation since the great dens sit at the loftiest peaks
	"den": {glyph: 'D', label: "dragon den", featureLabel: "dragon den",
		shading: boldShade(3000, 500, [5]string{"88", "124", "160", "196", "9"})},
	// nest — orange-rust, drake's lair; less imposing than the
	// dragon's blood-red but in the same warning-color family
	"nest": {glyph: 'd', label: "drake nest", featureLabel: "drake nest",
		shading: shade(500, 200, [5]string{"94", "130", "166", "172", "208"})},
	// rookery — sandy/dun, wyvern colonies on cliffs; muted earth
	// tones since wyverns are "the lesser raider" — common, less
	// fearsome than drakes or dragons
	"rookery": {glyph: 'w', label: "wyvern rookery", featureLabel: "wyvern rookery",
		shading: shade(2500, 200, [5]string{"94", "137", "143", "180", "187"})},
	"unknown": {glyph: '?', label: "unknown",
		shading: shade(0, 1, [5]string{"99", "99", "99", "99", "99"})},
	"drowned": {glyph: '_', label: "drowned",
		shading: shade(-800, 100, [5]string{"60", "60", "60", "60", "60"})},
}

// styleFor returns a lipgloss Style for InfoPanel rendering.
// The hot grid path uses colorFor + appendCell instead.
func styleFor(kind string, elev float64) lipgloss.Style {
	color, bold := colorFor(kind, elev)
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
	if bold {
		st = st.Bold(true)
	}
	return st
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

const (
	riverColor = "51"
	roadColor  = "180"
	pathColor  = "220" // amber gold — expedition trail
)

// PathCell is one step on an expedition route, with the directional glyph
// indicating which way the traveller moves to the next cell.
type PathCell struct {
	X, Y int64
	G    rune // from DirectionalGlyph; '@' for the start marker
}

var (
	riverStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(riverColor)).Bold(true)
	roadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(roadColor))
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	sepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

const (
	emptyGlyph = ' '
	riverGlyph = ','
	roadGlyph  = '·'
)

// bufCell stores the pre-computed render data for one map cell.
// Using (color, bold) instead of lipgloss.Style avoids per-cell style
// serialization in the hot render path — appendCell writes ANSI directly.
type bufCell struct {
	g     rune
	color string // ANSI-256 code, "" = no color
	bold  bool
}

// colorFor returns the ANSI-256 color code and bold flag for a grid cell.
// Used in the hot render path; allocates nothing.
func colorFor(kind string, elev float64) (color string, bold bool) {
	spec, ok := kinds[kind]
	if !ok {
		return "", false
	}
	s := spec.shading
	return s.colors[elevTier(elev, s.base, s.amp)], s.bold
}

// appendCell writes one ANSI-escaped glyph directly to b.
// reversed adds SGR 7 (reverse-video) for the cursor overlay.
func appendCell(b *strings.Builder, color string, bold, reversed bool, g rune) {
	if color == "" && !bold && !reversed {
		b.WriteRune(g)
		return
	}
	b.WriteString("\x1b[")
	first := true
	if color != "" {
		b.WriteString("38;5;")
		b.WriteString(color)
		first = false
	}
	if bold {
		if !first {
			b.WriteByte(';')
		}
		b.WriteByte('1')
		first = false
	}
	if reversed {
		if !first {
			b.WriteByte(';')
		}
		b.WriteByte('7')
	}
	b.WriteByte('m')
	b.WriteRune(g)
	b.WriteString("\x1b[0m")
}

// renderRow converts one row of bufCells to an ANSI string.
// curCol < 0 means no cursor in this row.
func renderRow(row []bufCell, curCol int) string {
	var b strings.Builder
	b.Grow(len(row) * 16)
	for j, c := range row {
		appendCell(&b, c.color, c.bold, j == curCol, c.g)
	}
	return b.String()
}

// GridBuf pre-renders the map to per-row strings. Built once per regen
// (the expensive path — all cells rendered); cursor moves re-render only
// the single affected row, making them ~50× faster.
type GridBuf struct {
	rows       []string    // pre-rendered ANSI row strings, no cursor
	raw        [][]bufCell // cell data kept for cursor-row re-render
	minX, minY int64
}

// BuildGridBuf constructs a GridBuf from query results. Called once per
// regen in the background; cursor updates call Render() which is fast.
func BuildGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64) *GridBuf {
	width := int(maxX - minX + 1)
	height := int(maxY - minY + 1)
	if width <= 0 || height <= 0 {
		return &GridBuf{minX: minX, minY: minY}
	}

	buf := make([][]bufCell, height)
	for i := range buf {
		buf[i] = make([]bufCell, width)
		for j := range buf[i] {
			buf[i][j] = bufCell{emptyGlyph, "", false}
		}
	}

	// Terrain pass.
	for _, c := range cells {
		gy, gx := int(c.Y-minY), int(c.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		g := '?'
		if spec, ok := kinds[c.Kind]; ok {
			g = spec.glyph
		}
		col, bd := colorFor(c.Kind, c.Elevation)
		buf[gy][gx] = bufCell{g, col, bd}
	}

	// Roads paint over terrain but under rivers.
	cellKind := make(map[[2]int64]string, len(cells))
	for _, c := range cells {
		cellKind[[2]int64{c.X, c.Y}] = c.Kind
	}
	for _, r := range roads {
		gy, gx := int(r.Y-minY), int(r.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		switch cellKind[[2]int64{r.X, r.Y}] {
		case "seat", "march", "headwater", "outhold", "reach":
			continue
		}
		buf[gy][gx] = bufCell{roadGlyph, roadColor, false}
	}

	// Rivers paint over roads and terrain.
	riverGlyphAt := make(map[[2]int64]rune, len(rivers))
	groups := make(map[int64][]db.GetRiverCellsInBoundsRow)
	for _, r := range rivers {
		groups[r.RiverID] = append(groups[r.RiverID], r)
	}
	for id := range groups {
		sort.Slice(groups[id], func(i, j int) bool { return groups[id][i].Ord < groups[id][j].Ord })
		group := groups[id]
		for i := range group {
			c := group[i]
			g := riverGlyph
			if i+1 < len(group) {
				next := group[i+1]
				g = DirectionalGlyph(int(next.X-c.X), int(next.Y-c.Y))
			}
			riverGlyphAt[[2]int64{c.X, c.Y}] = g
		}
	}
	for _, r := range rivers {
		gy, gx := int(r.Y-minY), int(r.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		g := riverGlyphAt[[2]int64{r.X, r.Y}]
		if g == 0 {
			g = riverGlyph
		}
		buf[gy][gx] = bufCell{g, riverColor, true}
	}

	// Pre-render all rows (no cursor).
	rows := make([]string, height)
	for i, row := range buf {
		rows[i] = renderRow(row, -1)
	}
	return &GridBuf{rows: rows, raw: buf, minX: minX, minY: minY}
}

// Render assembles the grid string with cursor and expedition path overlay.
// Only rows that need changes (cursor row or rows with path cells) are
// re-rendered; all other rows use the pre-rendered cached strings.
func (gb *GridBuf) Render(curX, curY int64, path []PathCell) string {
	if len(gb.rows) == 0 {
		return ""
	}
	height := len(gb.rows)
	curRow := -1
	if len(gb.raw) > 0 {
		cy := int(curY - gb.minY)
		cx := int(curX - gb.minX)
		if cy >= 0 && cy < height && cx >= 0 && cx < len(gb.raw[cy]) {
			curRow = cy
		}
	}

	// Index path cells by row → map[col]glyph for O(1) lookup.
	type entry struct {
		col int
		g   rune
	}
	pathOverlay := make(map[int][]entry, len(path))
	for _, p := range path {
		row := int(p.Y - gb.minY)
		col := int(p.X - gb.minX)
		if row >= 0 && row < height {
			pathOverlay[row] = append(pathOverlay[row], entry{col, p.G})
		}
	}

	var b strings.Builder
	b.Grow(height * len(gb.rows[0]) * 2)
	for i, row := range gb.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		overlay := pathOverlay[i]
		isCurRow := i == curRow
		if !isCurRow && len(overlay) == 0 {
			b.WriteString(row)
			continue
		}
		// Re-render this row with cursor and/or path overlay.
		curColInRow := -1
		if isCurRow {
			curColInRow = int(curX - gb.minX)
		}
		colGlyph := make(map[int]rune, len(overlay))
		for _, e := range overlay {
			colGlyph[e.col] = e.g
		}
		for j, c := range gb.raw[i] {
			isCursor := j == curColInRow
			if g, isPath := colGlyph[j]; isPath {
				appendCell(&b, pathColor, true, isCursor, g)
			} else {
				appendCell(&b, c.color, c.bold, isCursor, c.g)
			}
		}
	}
	return b.String()
}

// Grid renders the world map in one shot. Used by --print mode.
// Interactive TUI should use BuildGridBuf + Render for cursor performance.
func Grid(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, curX, curY int64) string {
	return BuildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY).Render(curX, curY, nil)
}

// CellInfo aggregates all known data about one map cell for display in
// the info panel. Only the fields that have data are populated — callers
// check SeatName/RiverName/FeatureName for non-empty to know what's there.
type CellInfo struct {
	Kind string
	Elev float64
	X, Y int64

	// Populated when the cell is a seat tier (seat/march/headwater/outhold/reach).
	SeatName     string
	SeatPressure float64

	// Populated when a river flows through this cell.
	RiverName string

	// Populated when the cell is a named feature (den/nest/rookery/lake/pass).
	FeatureName string
}

// labelOr returns label unless it's empty, in which case the raw kind
// string is the best display we have.
func labelOr(label, kind string) string {
	if label == "" {
		return kind
	}
	return label
}

func sep() string { return sepStyle.Render("   ·   ") }

// InfoPanel returns a single styled line describing the cell under the cursor.
func InfoPanel(info CellInfo) string {
	if info.Kind == "" {
		return dimStyle.Render(fmt.Sprintf("(%d, %d)", info.X, info.Y))
	}

	spec := kinds[info.Kind]
	var parts []string

	switch {
	case info.SeatName != "":
		// Named seat — lead with the hall name and tier.
		seatSt := styleFor(info.Kind, info.Elev)
		parts = append(parts, seatSt.Render(info.SeatName))
		parts = append(parts, seatSt.Render(labelOr(spec.tierLabel, info.Kind)))
		if info.SeatPressure > 0 {
			parts = append(parts, dimStyle.Render(fmt.Sprintf("dragon pressure %.0f", info.SeatPressure)))
		}
		if info.RiverName != "" {
			parts = append(parts, dimStyle.Render("river "+info.RiverName))
		}

	case info.FeatureName != "":
		// Named feature — lead with the procgen name and kind label.
		featSt := styleFor(info.Kind, info.Elev)
		parts = append(parts, featSt.Render(info.FeatureName))
		parts = append(parts, featSt.Render(labelOr(spec.featureLabel, info.Kind)))
		parts = append(parts, dimStyle.Render(fmt.Sprintf("elev %.0fm", info.Elev)))

	case info.RiverName != "":
		// River cell — show river name then underlying terrain.
		parts = append(parts, riverStyle.Render("river "+info.RiverName))
		parts = append(parts, dimStyle.Render(labelOr(spec.label, info.Kind)))
		parts = append(parts, dimStyle.Render(fmt.Sprintf("elev %.0fm", info.Elev)))

	default:
		// Plain terrain.
		terrSt := styleFor(info.Kind, info.Elev)
		parts = append(parts, terrSt.Render(labelOr(spec.label, info.Kind)))
		parts = append(parts, dimStyle.Render(fmt.Sprintf("elev %.0fm", info.Elev)))
	}

	return strings.Join(parts, sep())
}

func Title(s string) string {
	return titleStyle.Render(s)
}

func Legend() string {
	item := func(kind string, label string) string {
		spec := kinds[kind]
		s := spec.shading
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(s.colors[2]))
		if s.bold {
			st = st.Bold(true)
		}
		return st.Render(string(spec.glyph)) + dimStyle.Render(" "+label)
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
		item("marsh", "marsh"),
		item("seat", "tributary"),
	}, "   ")
	row5 := strings.Join([]string{
		item("march", "march"),
		item("headwater", "headwater"),
		item("outhold", "outhold"),
		item("reach", "reach"),
	}, "   ")
	row6 := strings.Join([]string{
		item("pass", "pass"),
		item("den", "dragon den"),
		item("nest", "drake nest"),
		item("rookery", "wyvern rookery"),
		roadStyle.Render(string(roadGlyph)) + dimStyle.Render(" road"),
	}, "   ")
	return row1 + "\n" + row2 + "\n" + row3 + "\n" + row4 + "\n" + row5 + "\n" + row6
}
