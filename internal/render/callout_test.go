package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

func TestCallout_Geometry(t *testing.T) {
	chip := Callout([]string{"short", "a much longer line of text"})
	if len(chip) != 2 {
		t.Fatalf("got %d lines, want 2", len(chip))
	}
	w := lipgloss.Width(chip[0])
	if lipgloss.Width(chip[1]) != w {
		t.Errorf("chip lines differ in width: %d vs %d", w, lipgloss.Width(chip[1]))
	}
	long := Callout([]string{strings.Repeat("x", 200)})
	if got := lipgloss.Width(long[0]); got > calloutMaxWidth+2 {
		t.Errorf("chip width %d exceeds max %d (+padding)", got, calloutMaxWidth+2)
	}
	if !strings.Contains(stripANSI(long[0]), "…") {
		t.Error("truncated chip missing ellipsis")
	}
}

func calloutFixture() *GridBuf {
	var cells []db.GetCellsInBoundsRow
	for y := int64(0); y < 10; y++ {
		for x := int64(0); x < 40; x++ {
			cells = append(cells, db.GetCellsInBoundsRow{X: x, Y: y, Kind: "cradle", Elevation: 100})
		}
	}
	return BuildGridBuf(cells, nil, nil, 0, 0, 39, 9)
}

func TestRenderWithCallouts_Placement(t *testing.T) {
	gb := calloutFixture()

	// Anchored mid-grid: the chip lands on the row below its anchor.
	out := stripANSI(gb.RenderWithCallouts(-1, -1, nil, []Overlay{
		{Lines: Callout([]string{"hello"}), X: 5, Y: 3},
	}))
	rows := strings.Split(out, "\n")
	if len(rows) != 10 {
		t.Fatalf("row count changed: %d", len(rows))
	}
	if !strings.Contains(rows[4], "hello") {
		t.Errorf("anchored chip not on row below anchor:\n%s", out)
	}
	for i, r := range rows {
		if w := lipgloss.Width(r); w != 40 {
			t.Errorf("row %d visible width %d, want 40", i, w)
		}
	}

	// Bottom edge: flips above the anchor.
	out = stripANSI(gb.RenderWithCallouts(-1, -1, nil, []Overlay{
		{Lines: Callout([]string{"flip"}), X: 5, Y: 9},
	}))
	rows = strings.Split(out, "\n")
	if !strings.Contains(rows[8], "flip") {
		t.Error("chip at the bottom edge should flip above its anchor")
	}

	// Right edge: clamps inside the grid.
	out = stripANSI(gb.RenderWithCallouts(-1, -1, nil, []Overlay{
		{Lines: Callout([]string{"clamped"}), X: 39, Y: 2},
	}))
	for i, r := range strings.Split(out, "\n") {
		if w := lipgloss.Width(r); w != 40 {
			t.Errorf("row %d width %d after clamping, want 40", i, w)
		}
	}

	// Top-right pin (toasts).
	out = stripANSI(gb.RenderWithCallouts(-1, -1, nil, []Overlay{
		{Lines: Callout([]string{"toast"}), TopRight: true},
	}))
	rows = strings.Split(out, "\n")
	if !strings.HasSuffix(rows[0], "toast ") {
		t.Errorf("toast not pinned top-right: %q", rows[0])
	}
}

func TestRenderWithCallouts_CollisionAndCursor(t *testing.T) {
	gb := calloutFixture()

	// Two chips on the same spot: first placed wins, second is dropped.
	out := stripANSI(gb.RenderWithCallouts(-1, -1, nil, []Overlay{
		{Lines: Callout([]string{"first"}), X: 5, Y: 3},
		{Lines: Callout([]string{"second"}), X: 6, Y: 3},
	}))
	if !strings.Contains(out, "first") {
		t.Error("first chip missing")
	}
	if strings.Contains(out, "second") {
		t.Error("overlapping second chip should have been dropped")
	}

	// The cursor stays visible on a row that also carries a chip.
	withCursor := gb.RenderWithCallouts(30, 4, nil, []Overlay{
		{Lines: Callout([]string{"chip"}), X: 5, Y: 3},
	})
	if !strings.Contains(withCursor, ";7m") && !strings.Contains(withCursor, "[7m") {
		t.Error("cursor reverse-video lost on a callout row")
	}
	// And the expedition path still draws around it.
	withPath := stripANSI(gb.RenderWithCallouts(-1, -1,
		[]PathCell{{X: 30, Y: 4, G: '@'}},
		[]Overlay{{Lines: Callout([]string{"chip"}), X: 5, Y: 3}}))
	if !strings.Contains(strings.Split(withPath, "\n")[4], "@") {
		t.Error("path glyph lost on a callout row")
	}
}
