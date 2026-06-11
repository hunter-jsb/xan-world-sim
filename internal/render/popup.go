package render

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Popup rendering — a bordered modal box overlaid on the map. The box
// is built as fixed-visible-width ANSI lines (PopupBox) and spliced
// into the grid by RenderWithOverlay, so the same primitive serves
// location dossiers, action menus, help, and expedition events.

var (
	popupBorderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	popupTitleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	popupOptStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	popupSelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
)

// popupMaxContentWidth keeps boxes comfortably inside the 120-col map.
const popupMaxContentWidth = 96

// popupMaxOptions is how many option rows render at once; longer lists
// scroll, windowed around the selection with above/below counters.
const popupMaxOptions = 12

// PopupBox renders a modal box as styled lines of equal visible width.
// body lines may carry their own ANSI styling. options render below a
// blank spacer; sel highlights one of them (pass -1 for none). The
// caller overlays the result with GridBuf.RenderWithOverlay.
func PopupBox(title string, body []string, options []string, sel int) []string {
	clip := func(s string) string {
		if lipgloss.Width(s) > popupMaxContentWidth {
			return truncateANSI(s, popupMaxContentWidth)
		}
		return s
	}
	title = clip(title)
	width := lipgloss.Width(title)
	clipped := make([]string, len(body))
	for i, l := range body {
		clipped[i] = clip(l)
		if w := lipgloss.Width(clipped[i]); w > width {
			width = w
		}
	}
	clippedOpts := make([]string, len(options))
	for i, o := range options {
		clippedOpts[i] = clip(o)
		if w := lipgloss.Width(clippedOpts[i]) + 2; w > width {
			width = w
		}
	}

	// Scroll window for long option lists, computed up front so the
	// indicator rows participate in the width calculation.
	start, end := 0, len(clippedOpts)
	var above, below string
	if end > popupMaxOptions {
		start = sel - popupMaxOptions/2
		if start < 0 {
			start = 0
		}
		if start > len(clippedOpts)-popupMaxOptions {
			start = len(clippedOpts) - popupMaxOptions
		}
		end = start + popupMaxOptions
		if start > 0 {
			above = fmt.Sprintf("  ⋯ %d above", start)
			if w := lipgloss.Width(above); w > width {
				width = w
			}
		}
		if end < len(clippedOpts) {
			below = fmt.Sprintf("  ⋯ %d below", len(clippedOpts)-end)
			if w := lipgloss.Width(below); w > width {
				width = w
			}
		}
	}

	pad := func(s string) string {
		return s + strings.Repeat(" ", width-lipgloss.Width(s))
	}
	row := func(content string) string {
		return popupBorderStyle.Render("│") + " " + pad(content) + " " + popupBorderStyle.Render("│")
	}

	out := make([]string, 0, len(body)+len(options)+4)
	out = append(out, popupBorderStyle.Render("┌"+strings.Repeat("─", width+2)+"┐"))
	out = append(out, row(popupTitleStyle.Render(title)))
	for _, l := range clipped {
		out = append(out, row(l))
	}
	if len(clippedOpts) > 0 {
		out = append(out, row(""))
		if above != "" {
			out = append(out, row(popupBorderStyle.Render(above)))
		}
		for i := start; i < end; i++ {
			if i == sel {
				out = append(out, row(popupSelStyle.Render("▸ "+clippedOpts[i])))
			} else {
				out = append(out, row(popupOptStyle.Render("  "+clippedOpts[i])))
			}
		}
		if below != "" {
			out = append(out, row(popupBorderStyle.Render(below)))
		}
	}
	out = append(out, popupBorderStyle.Render("└"+strings.Repeat("─", width+2)+"┘"))
	return out
}

// truncateANSI cuts a styled string to the given visible width,
// preserving escape sequences and appending a reset so trailing style
// can't bleed into the border.
func truncateANSI(s string, width int) string {
	var b strings.Builder
	visible := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := i
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		if visible >= width-1 {
			b.WriteString("…")
			break
		}
		// Copy one UTF-8 rune.
		r := []rune(s[i:])[0]
		b.WriteRune(r)
		i += len(string(r))
		visible++
	}
	return b.String() + "\x1b[0m"
}

// RenderWithOverlay renders the grid (with cursor and path) and then
// splices the overlay box, centered. Cells under and beside the box on
// affected rows are re-rendered from the raw buffer without cursor or
// path decoration — the popup is modal, so that simplification never
// shows.
func (gb *GridBuf) RenderWithOverlay(curX, curY int64, path []PathCell, overlay []string) string {
	base := gb.Render(curX, curY, path)
	if len(overlay) == 0 || len(gb.raw) == 0 {
		return base
	}
	rows := strings.Split(base, "\n")
	gridW := len(gb.raw[0])
	boxW := lipgloss.Width(overlay[0])
	x0 := (gridW - boxW) / 2
	if x0 < 0 {
		x0 = 0
	}
	y0 := (len(rows) - len(overlay)) / 2
	if y0 < 0 {
		y0 = 0
	}
	for i, ol := range overlay {
		r := y0 + i
		if r >= len(rows) {
			break
		}
		var b strings.Builder
		for j := 0; j < x0 && j < len(gb.raw[r]); j++ {
			c := gb.raw[r][j]
			appendCell(&b, c.color, c.bold, false, c.g)
		}
		b.WriteString(ol)
		for j := x0 + boxW; j < len(gb.raw[r]); j++ {
			c := gb.raw[r][j]
			appendCell(&b, c.color, c.bold, false, c.g)
		}
		rows[r] = b.String()
	}
	return strings.Join(rows, "\n")
}
