package main

import (
	"strings"
	"testing"

	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// TestChronicleEventPages: chronicle entries open event pages; pages
// carry the impact, a jump, the causal thread when one exists, and a
// way back that lands on the entry just read.
func TestChronicleEventPages(t *testing.T) {
	s := world.NewSim(42, 0)
	for y := 0; y < 600; y++ {
		s.StepYear()
	}
	m := &model{simMode: true, sim: s}

	m.openChroniclePopup(-1)
	if m.popup == nil || len(m.popup.opts) != len(s.Log) {
		t.Fatalf("chronicle lists %d entries, want %d", len(m.popup.opts), len(s.Log))
	}
	if m.popup.opts[0].action != popEventDetail {
		t.Fatalf("chronicle entries should open event pages, got %q", m.popup.opts[0].action)
	}

	// Find an event with a cause — the web must be browsable.
	caused := -1
	for i, e := range s.Log {
		if e.Cause >= 0 {
			caused = i
			break
		}
	}
	if caused < 0 {
		t.Fatal("no caused event in six centuries — the web never connected")
	}

	m.openEventDetailPopup(caused)
	if m.popup.opts[0].label != "Jump there" || m.popup.opts[0].action != popJumpXY {
		t.Errorf("event page's first option = %+v, want Jump there", m.popup.opts[0])
	}
	var hasThread, hasBack bool
	for _, o := range m.popup.opts {
		if o.action == popEventDetail && o.arg == s.Log[caused].Cause {
			hasThread = true
		}
		if o.action == popChronicle {
			hasBack = true
		}
	}
	if !hasThread {
		t.Error("caused event's page has no thread back to its cause")
	}
	if !hasBack {
		t.Error("event page has no way back to the chronicle")
	}
	joined := strings.Join(m.popup.body, "\n")
	if !strings.Contains(joined, "grown from") {
		t.Error("event page body doesn't show the cause line")
	}

	// Back lands on the entry just read (latest-first ordering).
	m.openChroniclePopup(m.lastEventIdx)
	if want := len(s.Log) - 1 - caused; m.popup.sel != want {
		t.Errorf("chronicle reopened at %d, want %d", m.popup.sel, want)
	}
}
