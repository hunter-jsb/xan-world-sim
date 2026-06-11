package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// TestKinds_CoversEveryRegionKind keeps the renderer's spec table in
// lockstep with the world package: every region the generator can emit
// must have a glyph, a label, and a full shading ramp.
func TestKinds_CoversEveryRegionKind(t *testing.T) {
	for id := int64(1); ; id++ {
		kind := world.RegionKind(id)
		if kind == "" {
			if id == 1 {
				t.Fatal("world.RegionKind(1) empty — region table missing?")
			}
			break // walked past the last region ID
		}
		spec, ok := kinds[kind]
		if !ok {
			t.Errorf("kind %q (region %d) missing from render kinds table", kind, id)
			continue
		}
		if spec.glyph == 0 {
			t.Errorf("kind %q has no glyph", kind)
		}
		if spec.label == "" {
			t.Errorf("kind %q has no label", kind)
		}
		for i, c := range spec.shading.colors {
			if c == "" {
				t.Errorf("kind %q shading color %d is empty", kind, i)
			}
		}
	}
}

func TestKinds_SeatAndFeatureLabels(t *testing.T) {
	for _, kind := range []string{"seat", "march", "headwater", "outhold", "reach", "capital"} {
		if kinds[kind].tierLabel == "" {
			t.Errorf("seat kind %q has no tierLabel", kind)
		}
	}
	for _, kind := range []string{"den", "nest", "rookery", "lake", "pass"} {
		if kinds[kind].featureLabel == "" {
			t.Errorf("feature kind %q has no featureLabel", kind)
		}
	}
}

func TestColorFor_UnknownKind(t *testing.T) {
	color, bold := colorFor("not_a_kind", 0)
	if color != "" || bold {
		t.Errorf("colorFor(unknown) = (%q, %v), want (\"\", false)", color, bold)
	}
}

func TestDirectionalGlyph(t *testing.T) {
	cases := []struct {
		dx, dy int
		want   rune
	}{
		{1, 0, '>'},
		{-1, 0, '<'},
		{0, 1, 'v'},
		{1, 1, '\\'},
		{-1, 1, '/'},
		{1, -1, '/'},
		{-1, -1, '\\'},
		{0, 0, ','},
		{0, -1, ','}, // straight north has no dedicated glyph
	}
	for _, c := range cases {
		if got := DirectionalGlyph(c.dx, c.dy); got != c.want {
			t.Errorf("DirectionalGlyph(%d,%d) = %q, want %q", c.dx, c.dy, got, c.want)
		}
	}
}

// stripANSI removes SGR escape sequences so tests can assert on glyphs.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestGridBuf_RenderWithCursorAndPath(t *testing.T) {
	cells := []db.GetCellsInBoundsRow{
		{X: 0, Y: 0, Kind: "cradle", Elevation: 100},
		{X: 1, Y: 0, Kind: "mountain", Elevation: 3000},
		{X: 0, Y: 1, Kind: "sea_brine", Elevation: -800},
		{X: 1, Y: 1, Kind: "forest", Elevation: 120},
	}
	gb := BuildGridBuf(cells, nil, nil, 0, 0, 1, 1)

	plain := stripANSI(gb.Render(-1, -1, nil))
	rows := strings.Split(plain, "\n")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0] != ".A" || rows[1] != "%T" {
		t.Errorf("rendered grid = %q, %q; want \".A\", \"%%T\"", rows[0], rows[1])
	}

	// Cursor overlay adds reverse-video on the cursor cell but must
	// leave the glyphs unchanged.
	withCursor := gb.Render(1, 1, nil)
	if stripANSI(withCursor) != plain {
		t.Errorf("cursor render changed glyphs: %q vs %q", stripANSI(withCursor), plain)
	}
	if !strings.Contains(withCursor, ";7m") && !strings.Contains(withCursor, "[7m") {
		t.Error("cursor render contains no reverse-video escape")
	}

	// Path overlay replaces glyphs along the route.
	path := []PathCell{{X: 0, Y: 0, G: '@'}, {X: 1, Y: 1, G: 'X'}}
	withPath := stripANSI(gb.Render(-1, -1, path))
	rows = strings.Split(withPath, "\n")
	if rows[0] != "@A" || rows[1] != "%X" {
		t.Errorf("path render = %q, %q; want \"@A\", \"%%X\"", rows[0], rows[1])
	}
}

func TestGridBuf_EmptyViewport(t *testing.T) {
	gb := BuildGridBuf(nil, nil, nil, 0, 0, -1, -1)
	if got := gb.Render(0, 0, nil); got != "" {
		t.Errorf("empty viewport rendered %q, want empty string", got)
	}
}

func TestPopupBox_Geometry(t *testing.T) {
	box := PopupBox("Title", []string{"a body line", "x"}, []string{"Opt A", "Option Bee"}, 1)
	if len(box) != 2+1+2+1+2 { // borders + title + body + spacer + options
		t.Fatalf("got %d lines, want 8", len(box))
	}
	w := lipgloss.Width(box[0])
	for i, l := range box {
		if lw := lipgloss.Width(l); lw != w {
			t.Errorf("line %d width %d != box width %d (%q)", i, lw, w, stripANSI(l))
		}
	}
	plain := stripANSI(strings.Join(box, "\n"))
	for _, want := range []string{"Title", "a body line", "▸ Option Bee", "  Opt A", "┌", "└"} {
		if !strings.Contains(plain, want) {
			t.Errorf("box missing %q:\n%s", want, plain)
		}
	}
}

func TestPopupBox_TruncatesLongContent(t *testing.T) {
	long := strings.Repeat("x", 300)
	box := PopupBox("t", []string{long}, nil, -1)
	for i, l := range box {
		if w := lipgloss.Width(l); w > popupMaxContentWidth+4 {
			t.Errorf("line %d visible width %d exceeds max box width", i, w)
		}
	}
	if !strings.Contains(stripANSI(strings.Join(box, "")), "…") {
		t.Error("truncated content missing ellipsis")
	}
}

func TestGridBuf_RenderWithOverlay(t *testing.T) {
	var cells []db.GetCellsInBoundsRow
	for y := int64(0); y < 9; y++ {
		for x := int64(0); x < 30; x++ {
			cells = append(cells, db.GetCellsInBoundsRow{X: x, Y: y, Kind: "cradle", Elevation: 100})
		}
	}
	gb := BuildGridBuf(cells, nil, nil, 0, 0, 29, 8)
	box := PopupBox("Hi", []string{"body"}, nil, -1)
	out := stripANSI(gb.RenderWithOverlay(-1, -1, nil, box))
	rows := strings.Split(out, "\n")
	if len(rows) != 9 {
		t.Fatalf("overlay changed row count: %d", len(rows))
	}
	// The box is centered: middle rows contain its borders and text.
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"┌", "└", "Hi", "body"} {
		if !strings.Contains(joined, want) {
			t.Errorf("overlay output missing %q:\n%s", want, joined)
		}
	}
	// Rows outside the box are untouched terrain.
	if !strings.HasPrefix(rows[0], "...") {
		t.Errorf("top row should be terrain, got %q", rows[0])
	}
	// Every row keeps the grid's visible width.
	for i, r := range rows {
		if w := lipgloss.Width(r); w != 30 {
			t.Errorf("row %d visible width %d, want 30", i, w)
		}
	}
}
