package render

import (
	"strings"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

var glyphForKind = map[string]rune{
	"plateau":     '^',
	"mountain":    'A',
	"cradle":      '.',
	"doab":        '#',
	"sea_brine":   '%',
	"sea_eastern": '~',
	"unknown":     '?',
	"drowned":     '_',
}

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
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = emptyGlyph
		}
	}
	// Layer 1: regions
	for _, c := range cells {
		gy, gx := int(c.Y-minY), int(c.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		if g, ok := glyphForKind[c.Kind]; ok {
			grid[gy][gx] = g
		} else {
			grid[gy][gx] = '?'
		}
	}
	// Layer 2: rivers (overlaid)
	for _, r := range rivers {
		gy, gx := int(r.Y-minY), int(r.X-minX)
		if gy < 0 || gy >= height || gx < 0 || gx >= width {
			continue
		}
		grid[gy][gx] = riverGlyph
	}

	var b strings.Builder
	for i, row := range grid {
		b.WriteString(string(row))
		if i < len(grid)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func Legend() string {
	return strings.Join([]string{
		"^ plateau    A mountain   . cradle      # doab",
		"% brine      ~ eastern    , river       ? unknown",
	}, "\n")
}
