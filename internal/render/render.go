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
	"agraria":        ';', // coast: lower shelf, marshy
	"agraria_upland": '\'', // upland: higher, drier, exposes first
	"unknown":        '?',
	"drowned":        '_',
}

var styleForKind = map[string]lipgloss.Style{
	"plateau":     lipgloss.NewStyle().Foreground(lipgloss.Color("253")), // light gray (snow)
	"mountain":    lipgloss.NewStyle().Foreground(lipgloss.Color("244")), // medium gray (stone)
	"foothill":    lipgloss.NewStyle().Foreground(lipgloss.Color("100")), // olive (rolling)
	"cliff":       lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true),
	"cradle":      lipgloss.NewStyle().Foreground(lipgloss.Color("28")),  // green (fertile)
	"doab":        lipgloss.NewStyle().Foreground(lipgloss.Color("94")),  // brown (rugged)
	"sea_brine":   lipgloss.NewStyle().Foreground(lipgloss.Color("19")),  // deep blue (saline)
	"sea_eastern": lipgloss.NewStyle().Foreground(lipgloss.Color("38")),  // cyan (fresh)
	"glacier":        lipgloss.NewStyle().Foreground(lipgloss.Color("159")).Bold(true), // pale icy
	"agraria":        lipgloss.NewStyle().Foreground(lipgloss.Color("143")), // muted yellow-tan (lower coast — wetter)
	"agraria_upland": lipgloss.NewStyle().Foreground(lipgloss.Color("179")), // brighter tan (upland — drier, grass)
	"unknown":        lipgloss.NewStyle().Foreground(lipgloss.Color("99")),  // purple
	"drowned":        lipgloss.NewStyle().Foreground(lipgloss.Color("60")),  // muted blue
}

var (
	emptyStyle = lipgloss.NewStyle()
	riverStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true) // bright cyan
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
	// Layer 1: regions
	for _, c := range cells {
		gy, gx := int(c.Y-minY), int(c.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		g, ok := glyphForKind[c.Kind]
		if !ok {
			g = '?'
		}
		style, ok := styleForKind[c.Kind]
		if !ok {
			style = emptyStyle
		}
		grid[gy][gx] = style.Render(string(g))
	}
	// Layer 2: rivers (overlaid)
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
		st := styleForKind[kind]
		return st.Render(string(g)) + dimStyle.Render(" "+label)
	}
	row1 := strings.Join([]string{
		item("plateau", "plateau"),
		item("mountain", "mountain"),
		item("foothill", "foothill"),
		item("cliff", "cliff"),
		item("doab", "doab"),
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
	}, "   ")
	return row1 + "\n" + row2 + "\n" + row3
}
