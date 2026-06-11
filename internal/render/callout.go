package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Callouts are the small floating layer between the map and the modal
// popup: event tags pinned to the cells where news broke, toasts in
// the corner, tooltips beside the cursor. Unlike PopupBox they are
// non-modal — several render at once, and the cursor and expedition
// path keep drawing around them.

// calloutMaxWidth keeps chips compact — these annotate the map, they
// don't replace it.
const calloutMaxWidth = 38

var calloutStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("236")).
	Foreground(lipgloss.Color("252"))

// Callout renders plain-text lines as a compact chip: uniform visible
// width, one space of padding, dark background. Lines must be
// unstyled — the chip owns its styling (ANSI inside would tear the
// background).
func Callout(lines []string) []string {
	width := 0
	clipped := make([]string, len(lines))
	for i, l := range lines {
		r := []rune(l)
		if len(r) > calloutMaxWidth {
			r = append(r[:calloutMaxWidth-1], '…')
		}
		clipped[i] = string(r)
		if len(r) > width {
			width = len(r)
		}
	}
	out := make([]string, len(clipped))
	for i, l := range clipped {
		out[i] = calloutStyle.Render(" " + l + strings.Repeat(" ", width-len([]rune(l))) + " ")
	}
	return out
}

// Overlay is one floating chip with its place in the world: anchored
// to a cell (placed just below it, flipping above at the bottom edge,
// clamped horizontally) or pinned to the grid's top-right (toasts).
type Overlay struct {
	Lines    []string
	X, Y     int64 // anchor cell in world coords (ignored when TopRight)
	TopRight bool
}

// calloutSeg is one chip line placed on one grid row.
type calloutSeg struct {
	col   int
	width int
	line  string
}

// RenderWithCallouts renders the grid with cursor and path, then
// floats the overlays. Placement is first-come-first-kept: an overlay
// that would cover an earlier one is dropped — so callers order by
// importance (tooltip, toast, then event tags newest-first).
func (gb *GridBuf) RenderWithCallouts(curX, curY int64, path []PathCell, overlays []Overlay) string {
	height := len(gb.rows)
	if height == 0 || len(gb.raw) == 0 {
		return ""
	}
	gridW := len(gb.raw[0])

	segs := make(map[int][]calloutSeg, len(overlays)*2)
	type span struct{ c0, c1, r0, r1 int }
	var placed []span
	for _, o := range overlays {
		if len(o.Lines) == 0 {
			continue
		}
		w := lipgloss.Width(o.Lines[0])
		h := len(o.Lines)
		if w > gridW || h > height {
			continue
		}
		var row0, col0 int
		if o.TopRight {
			row0, col0 = 0, gridW-w
		} else {
			ax := int(o.X - gb.minX)
			ay := int(o.Y - gb.minY)
			col0 = min(max(ax, 0), gridW-w)
			row0 = ay + 1 // prefer just below the anchor
			if row0+h > height {
				row0 = ay - h // flip above
			}
			if row0 < 0 || row0+h > height {
				continue
			}
		}
		overlaps := false
		for _, s := range placed {
			if row0 < s.r1 && row0+h > s.r0 && col0 < s.c1 && col0+w > s.c0 {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		placed = append(placed, span{col0, col0 + w, row0, row0 + h})
		for i, l := range o.Lines {
			r := row0 + i
			segs[r] = append(segs[r], calloutSeg{col: col0, width: w, line: l})
		}
	}
	if len(placed) == 0 {
		return gb.Render(curX, curY, path)
	}

	curRow, curCol := -1, -1
	if cy, cx := int(curY-gb.minY), int(curX-gb.minX); cy >= 0 && cy < height && cx >= 0 && cx < gridW {
		curRow, curCol = cy, cx
	}
	type entry struct {
		col int
		g   rune
		dim bool
		hot bool
	}
	pathOverlay := make(map[int][]entry, len(path))
	for _, p := range path {
		row := int(p.Y - gb.minY)
		if row >= 0 && row < height {
			pathOverlay[row] = append(pathOverlay[row], entry{int(p.X - gb.minX), p.G, p.Dim, p.Hot})
		}
	}

	var b strings.Builder
	b.Grow(height * len(gb.rows[0]) * 2)
	for i, row := range gb.rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		rowSegs := segs[i]
		overlay := pathOverlay[i]
		if len(rowSegs) == 0 && i != curRow && len(overlay) == 0 {
			b.WriteString(row)
			continue
		}
		segAt := make(map[int]calloutSeg, len(rowSegs))
		for _, s := range rowSegs {
			segAt[s.col] = s
		}
		colEntry := make(map[int]entry, len(overlay))
		for _, e := range overlay {
			colEntry[e.col] = e
		}
		for j := 0; j < len(gb.raw[i]); j++ {
			if s, ok := segAt[j]; ok {
				b.WriteString(s.line)
				j += s.width - 1
				continue
			}
			c := gb.raw[i][j]
			isCursor := i == curRow && j == curCol
			if e, isPath := colEntry[j]; isPath {
				color := pathColor
				switch {
				case e.hot:
					color = pathHotColor
				case e.dim:
					color = pathDimColor
				}
				g := e.g
				if g == 0 {
					g = c.g
				}
				appendCell(&b, color, !e.dim, isCursor, g)
				continue
			}
			appendCell(&b, c.color, c.bold, isCursor, c.g)
		}
	}
	return b.String()
}
