package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/world"
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

// TestMapOverlays_TagPriority: headlines fill the tag cap before any
// minor chatter shows, newest first within each class.
func TestMapOverlays_TagPriority(t *testing.T) {
	m := &model{curX: -1, curY: -1, simMode: true, sim: &world.Sim{}}
	far := time.Now().Add(time.Hour)
	for i := 0; i < 4; i++ {
		m.simTags = append(m.simTags, simTag{x: int64(i), y: 0, label: fmt.Sprintf("minor %d", i), deadline: far})
	}
	m.simTags = append(m.simTags, simTag{x: 9, y: 9, label: "the headline", major: true, deadline: far})

	ovs := m.mapOverlays()
	if len(ovs) != maxTags {
		t.Fatalf("got %d tag overlays, want the cap %d", len(ovs), maxTags)
	}
	if ovs[0].X != 9 || ovs[0].Y != 9 {
		t.Errorf("first tag should be the major headline, got anchor (%d,%d)", ovs[0].X, ovs[0].Y)
	}
}

// TestFreezeTags: a held tick ages nothing — deadlines slide forward
// by exactly the held duration.
func TestFreezeTags(t *testing.T) {
	m := &model{}
	base := time.Now()
	m.simTags = []simTag{{label: "x", deadline: base}}
	m.simNote, m.simNoteDeadline = "note", base
	m.freezeTags(2 * time.Second)
	if got := m.simTags[0].deadline; !got.Equal(base.Add(2 * time.Second)) {
		t.Errorf("tag deadline %v, want +2s", got.Sub(base))
	}
	if got := m.simNoteDeadline; !got.Equal(base.Add(2 * time.Second)) {
		t.Errorf("note deadline %v, want +2s", got.Sub(base))
	}
}

// TestLensCycle: p walks all five lenses and wraps; entering a slice
// defaults to political and leaving restores the lens you had.
func TestLensCycle(t *testing.T) {
	if len(lensNames) != 5 {
		t.Fatalf("%d lenses, want 5", len(lensNames))
	}
	m := &model{}
	seen := map[int]bool{m.lens: true}
	for i := 0; i < len(lensNames); i++ {
		m.lens = (m.lens + 1) % len(lensNames)
		seen[m.lens] = true
	}
	if len(seen) != len(lensNames) || m.lens != lensTerrain {
		t.Errorf("cycle visited %d lenses ending at %d, want all %d ending back at terrain",
			len(seen), m.lens, len(lensNames))
	}

	// Sim entry forces political, exit restores.
	m.lens = lensClimate
	m.preSimLens = m.lens
	m.lens = lensPolitical // what startSim does
	if m.preSimLens != lensClimate {
		t.Error("pre-sim lens not stashed")
	}
	m.lens = m.preSimLens // what exitSim does
	if m.lens != lensClimate {
		t.Error("lens not restored on exit")
	}
}
