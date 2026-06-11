package render

import (
	"strings"
	"testing"

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
	for _, kind := range []string{"seat", "march", "headwater", "outhold", "reach"} {
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
