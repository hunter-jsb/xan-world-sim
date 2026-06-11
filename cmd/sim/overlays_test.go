package main

import (
	"strings"
	"testing"

	"github.com/hunterjsb/xan-world-sim/internal/db"
)

// TestMapOverlays: the floating layer assembles in priority order —
// tooltip at the cursor, toast in the corner, then the newest event
// tags capped at maxTags; expired labels and plain raid pings don't
// tag.
func TestMapOverlays(t *testing.T) {
	m := &model{curX: 5, curY: 5}
	m.seatAt = map[[2]int64]db.GetSeatsInBoundsRow{
		{5, 5}: {Name: "Palopar", Tier: "march", RealmName: "Thalor", Allegiance: 0.8},
	}
	m.featureAt = map[[2]int64]db.GetNamedFeaturesInBoundsRow{}

	// Tooltip alone.
	ovs := m.mapOverlays()
	if len(ovs) != 1 || ovs[0].TopRight {
		t.Fatalf("want 1 anchored tooltip overlay, got %d", len(ovs))
	}
	if ovs[0].X != 5 || ovs[0].Y != 5 {
		t.Errorf("tooltip anchored at (%d,%d), want the cursor", ovs[0].X, ovs[0].Y)
	}

	// Tooltip content covers name, kind, stance.
	lines := m.tooltipLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "Palopar") ||
		!strings.Contains(lines[0], "March") || !strings.Contains(lines[0], "sworn") {
		t.Errorf("tooltip = %q, want name · kind · stance", lines)
	}

	// Toast joins, pinned to the corner.
	m.toastText = "the years run at 2×"
	ovs = m.mapOverlays()
	if len(ovs) != 2 || !ovs[1].TopRight {
		t.Fatalf("want tooltip + corner toast, got %+v", ovs)
	}
}
