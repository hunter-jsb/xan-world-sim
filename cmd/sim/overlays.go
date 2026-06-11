package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hunterjsb/xan-world-sim/internal/render"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// The callout layer: toasts (transient corner notices), the cursor
// tooltip (named places announce themselves on the map), and in-map
// event tags (headlines pinned where they happened). All built on
// render.Callout / RenderWithCallouts; the modal popup still owns the
// screen when open.

// toastFor is how long a toast lingers before fading.
const toastFor = 2500 * time.Millisecond

// toastMsg clears an expired toast; gen guards stale timers.
type toastMsg struct{ gen int }

// showToast raises a corner notice (mirrored to the status line) and
// schedules its fade. A newer toast replaces the old and its timer.
func (m *model) showToast(text string) tea.Cmd {
	m.toastText = text
	m.status = text
	m.toastGen++
	gen := m.toastGen
	m.mapStr = m.buildMap()
	return tea.Tick(toastFor, func(time.Time) tea.Msg { return toastMsg{gen: gen} })
}

// tooltipLines names the place under the cursor — nil when the cell
// has nothing named on it.
func (m *model) tooltipLines() []string {
	k := [2]int64{m.curX, m.curY}
	if s, ok := m.seatAt[k]; ok {
		line := s.Name + " · " + render.KindDisplay(s.Tier)
		if s.RealmName != "" {
			line += " · " + world.AllegianceStance(s.Allegiance)
		}
		return []string{line}
	}
	if f, ok := m.featureAt[k]; ok {
		return []string{f.Name + " · " + render.KindDisplay(f.Kind)}
	}
	return nil
}

// mapOverlays assembles the floating layer in priority order —
// placement is first-come-first-kept, so the tooltip outranks the
// toast, which outranks event tags (newest first, capped).
func (m *model) mapOverlays() []render.Overlay {
	var out []render.Overlay
	if lines := m.tooltipLines(); len(lines) > 0 {
		out = append(out, render.Overlay{Lines: render.Callout(lines), X: m.curX, Y: m.curY})
	}
	if m.toastText != "" {
		out = append(out, render.Overlay{Lines: render.Callout([]string{m.toastText}), TopRight: true})
	}
	if m.simMode && m.sim != nil {
		shown := 0
		for i := len(m.simPings) - 1; i >= 0 && shown < maxTags; i-- {
			p := m.simPings[i]
			if p.label == "" || m.sim.Year >= p.labelUntil {
				continue
			}
			out = append(out, render.Overlay{Lines: render.Callout([]string{p.label}), X: p.x, Y: p.y})
			shown++
		}
	}
	return out
}
