package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// Simulation mode pins the current kya as a slice of deep time and
// runs years inside it. Deep time scrubs *between* equilibrium
// snapshots; the sim animates one of them — geography holds still
// while politics live. The engine (world.Sim) owns all dynamics; this
// file owns the clock, the overlay of sim state onto the render data,
// and the mode's keys. The sim never touches the database: the slice
// is ephemeral and deterministic, so leaving and re-entering replays
// the same history.

// simSpeeds are the wall-clock lengths of one simulated year; names
// index-match for the header readout. The brackets drive these — the
// same keys that pan deep time throttle the year clock inside a
// slice, so "brackets drive time" holds in both modes.
var (
	simSpeeds     = []time.Duration{600 * time.Millisecond, 300 * time.Millisecond, 150 * time.Millisecond, 75 * time.Millisecond, 37 * time.Millisecond}
	simSpeedNames = []string{"½×", "1×", "2×", "4×", "8×"}
)

// adjustSimSpeed nudges the year clock by one notch (dir ±1).
func (m *model) adjustSimSpeed(dir int) {
	next := m.simSpeed + dir
	switch {
	case next < 0:
		m.status = "the years already crawl (" + simSpeedNames[0] + ")"
		return
	case next >= len(simSpeeds):
		m.status = "the years already race (" + simSpeedNames[len(simSpeeds)-1] + ")"
		return
	}
	m.simSpeed = next
	m.status = "the years run at " + simSpeedNames[m.simSpeed]
}

// setSimSpeed jumps straight to a notch — the braces snap to the
// ends of the ladder like they take the big steps in deep time.
func (m *model) setSimSpeed(i int) {
	m.simSpeed = i
	m.status = "the years run at " + simSpeedNames[m.simSpeed]
}

// simTickMsg advances the year clock; gen guards against stale ticks
// after the mode is left.
type simTickMsg struct{ gen int }

// simReadyMsg delivers a freshly built simulation. seed/kya let the
// handler discard a sim built for a moment the user has scrubbed away
// from while it was generating.
type simReadyMsg struct {
	sim  *world.Sim
	gen  int
	seed int64
	kya  int
}

// enterSimCmd builds the slice's simulation off the Update loop —
// NewSim regenerates the world, which takes regen-scale time.
func (m *model) enterSimCmd() tea.Cmd {
	m.simGen++
	gen := m.simGen
	seed, kya := m.seed, m.kya
	m.status = "pinning the slice — the courts convene..."
	return func() tea.Msg {
		return simReadyMsg{sim: world.NewSim(seed, kya), gen: gen, seed: seed, kya: kya}
	}
}

// startSim installs a ready simulation: deep-time rows stashed,
// political view on (politics is what moves), sim state overlaid on
// the render data, clock running.
func (m *model) startSim(sim *world.Sim) tea.Cmd {
	m.simMode = true
	m.sim = sim
	m.simPaused = false
	m.simSpeed = 1
	m.stashDeepTime()
	m.preSimPolitical = m.politicalMode
	m.politicalMode = true
	m.applySimData(true)
	m.status = "the slice is pinned — years pass; space pauses, S returns to deep time"
	m.mapStr = m.buildMap()
	return m.simTickCmd()
}

// exitSim restores the deep-time equilibrium from the stash — the sim
// never wrote to the DB, so the slice's static world is exactly what
// it was. The chronicle goes with it; re-entering replays the same
// history. An expedition afield survives: it lives in the same frozen
// moment, the sim only animated its politics.
func (m *model) exitSim() {
	m.simMode = false
	m.sim = nil
	m.simGen++ // invalidate in-flight year ticks
	m.simPaused = false
	m.simPings = nil
	m.simNote, m.simNoteUntil = "", 0
	m.lastEventIdx = 0
	m.politicalMode = m.preSimPolitical
	m.data.seats = m.preSimSeats
	m.data.territory = m.preSimTerritory
	m.data.cells = m.preSimCells
	m.data.features = m.preSimFeatures
	m.preSimSeats, m.preSimTerritory = nil, nil
	m.preSimCells, m.preSimFeatures = nil, nil
	m.buildLookups()
	m.rebuildGrid()
	m.status = "returned to deep time"
}

// stashDeepTime keeps the deep-time rows the sim will overlay. Seats,
// territory, and features are replaced wholesale by applySimData, so
// references suffice; cells are patched element-wise (halls razed or
// raised), so the working copy is a clone.
func (m *model) stashDeepTime() {
	m.preSimSeats = m.data.seats
	m.preSimTerritory = m.data.territory
	m.preSimFeatures = m.data.features
	m.preSimCells = m.data.cells
	m.data.cells = append([]db.GetCellsInBoundsRow(nil), m.data.cells...)
	m.simPatchesApplied = 0
	m.simRuinCount = -1   // force the first features build
	m.simTerrVersion = -1 // force the first territory build
}

// simTickCmd schedules the next simulated year at the current speed.
func (m *model) simTickCmd() tea.Cmd {
	gen := m.simGen
	return tea.Tick(simSpeeds[m.simSpeed], func(time.Time) tea.Msg { return simTickMsg{gen: gen} })
}

// simPing is one alarm mark on the map: an event happened at (x, y)
// and the cell stays tinted until the given year.
type simPing struct {
	x, y  int64
	until int
}

const (
	pingYears = 10 // how long an event tints its cell
	newsYears = 8  // how long a headline holds the status line
)

// handleSimTick advances one year unless something holds the clock —
// an open popup (read in peace), an expedition afield (the court holds
// its breath, the day clock runs instead), or an explicit pause. The
// tick chain itself always continues while the mode is active.
//
// Events never pause the simulation: headlines take the status line
// for a few years and ping the map where they happened; the chronicle
// (L) keeps the full record, g jumps to the latest news.
func (m *model) handleSimTick(msg simTickMsg) tea.Cmd {
	if !m.simMode || msg.gen != m.simGen {
		return nil // stale tick from a left simulation
	}
	if m.popup == nil && m.exp == nil && !m.simPaused && m.sim != nil {
		events := m.sim.StepYear()
		var lastMajor *world.SimEvent
		minor := ""
		for i := range events {
			e := &events[i]
			if e.Major || e.Kind == "raid" {
				m.simPings = append(m.simPings, simPing{x: e.X, y: e.Y, until: m.sim.Year + pingYears})
			}
			if e.Major {
				lastMajor = e
			} else {
				minor = e.Text
			}
		}
		live := m.simPings[:0]
		for _, p := range m.simPings {
			if m.sim.Year < p.until {
				live = append(live, p)
			}
		}
		m.simPings = live
		m.applySimData(false)
		if lastMajor != nil {
			m.simNote = lastMajor.Text
			m.simNoteUntil = m.sim.Year + newsYears
		}
		switch {
		case m.simNote != "" && m.sim.Year < m.simNoteUntil:
			m.status = fmt.Sprintf("year %d ⚑ %s", m.sim.Year, m.simNote)
		case minor != "":
			m.status = fmt.Sprintf("year %d — %s", m.sim.Year, minor)
		default:
			m.status = fmt.Sprintf("year %d", m.sim.Year)
		}
		m.mapStr = m.buildMap()
	}
	return m.simTickCmd()
}

// applySimData overlays the simulation's political state onto the
// render data, in the same row shapes the DB queries produce. Seats
// change every year (allegiance, pressure); territory only when the
// engine's borders re-settled (its version stamp moves), so the grid
// rebuild is gated on that. force rebuilds everything (mode entry,
// stash refresh).
func (m *model) applySimData(force bool) {
	if m.sim == nil {
		return
	}
	territoryChanged := force || m.sim.TerritoryVersion() != m.simTerrVersion
	m.simTerrVersion = m.sim.TerritoryVersion()
	w := m.sim.W
	realmByID := make(map[int64]world.Realm, len(w.Realms))
	for _, r := range w.Realms {
		realmByID[r.ID] = r
	}
	seats := make([]db.GetSeatsInBoundsRow, len(w.Seats))
	for i, s := range w.Seats {
		row := db.GetSeatsInBoundsRow{
			X: s.X, Y: s.Y,
			Tier:       world.RegionKind(s.Tier),
			Name:       s.Name,
			Pressure:   s.Pressure,
			Allegiance: s.Allegiance,
			RealmID:    s.RealmID,
		}
		if r, ok := realmByID[s.RealmID]; ok {
			row.RealmName = r.Name
			row.IsCrown = r.IsCrown
		}
		seats[i] = row
	}
	m.data.seats = seats
	if territoryChanged {
		terr := make([]db.GetTerritoryInBoundsRow, len(w.Territory))
		for i, tc := range w.Territory {
			row := db.GetTerritoryInBoundsRow{
				X: tc.X, Y: tc.Y, RealmID: tc.RealmID,
				Contested: m.sim.Contested(tc.X, tc.Y),
			}
			if r, ok := realmByID[tc.RealmID]; ok {
				row.RealmName = r.Name
				row.IsCrown = r.IsCrown
			}
			terr[i] = row
		}
		m.data.territory = terr
	}

	// Rise and fall: splice new map-cell patches (halls razed or
	// raised) into the cloned cells, and rebuild the features list
	// when the set of ruins moved.
	gridDirty := false
	patches := m.sim.CellPatches()
	for ; m.simPatchesApplied < len(patches); m.simPatchesApplied++ {
		p := patches[m.simPatchesApplied]
		for ci := range m.data.cells {
			if m.data.cells[ci].X == p.X && m.data.cells[ci].Y == p.Y {
				m.data.cells[ci].Kind = p.Kind
				break
			}
		}
		gridDirty = true
	}
	if ruins := m.sim.Ruins(); len(ruins) != m.simRuinCount {
		m.simRuinCount = len(ruins)
		feats := append([]db.GetNamedFeaturesInBoundsRow(nil), m.preSimFeatures...)
		for _, r := range ruins {
			feats = append(feats, db.GetNamedFeaturesInBoundsRow{
				X: r.X, Y: r.Y, Kind: "ruin", Name: r.Name,
				Detail: fmt.Sprintf("sacked in year %d", r.Year),
			})
		}
		m.data.features = feats
	}

	m.buildLookups()
	if territoryChanged || gridDirty {
		m.rebuildGrid()
	}
}

// openSimExitPopup confirms leaving — the year counter is the one
// thing a slice doesn't keep.
func (m *model) openSimExitPopup() {
	year := 0
	if m.sim != nil {
		year = m.sim.Year
	}
	m.popup = &popupState{
		title: "leave the simulation?",
		body: []string{
			dimStyle.Render(fmt.Sprintf("%d years have passed in this slice.", year)),
			dimStyle.Render("deep time resumes; re-entering replays the same history from year 0."),
		},
		opts: []popupOption{
			{label: "Return to deep time", action: popExitSim},
			{label: "Stay", action: popClose},
		},
	}
	m.mapStr = m.buildMap()
}

// openChroniclePopup lists the slice's whole history, latest first;
// choosing an entry opens its page (impact, causes, jump). selIdx
// positions the selection on a chronicle index (-1 = the latest).
func (m *model) openChroniclePopup(selIdx int) {
	if !m.simMode || m.sim == nil {
		m.status = "the chronicle is written inside a simulation (S enters one)"
		return
	}
	log := m.sim.Log
	if len(log) == 0 {
		m.status = "nothing yet recorded in this slice"
		return
	}
	opts := make([]popupOption, 0, len(log))
	for i := len(log) - 1; i >= 0; i-- {
		e := log[i]
		marker := "  "
		if e.Major {
			marker = "⚑ "
		}
		opts = append(opts, popupOption{
			label:  fmt.Sprintf("y%4d %s%s", e.Year, marker, e.Text),
			action: popEventDetail,
			arg:    i,
		})
	}
	sel := 0
	if selIdx >= 0 && selIdx < len(log) {
		sel = len(log) - 1 - selIdx
	}
	m.popup = &popupState{
		title: fmt.Sprintf("chronicle — year %d, %d entries", m.sim.Year, len(log)),
		opts:  opts,
		sel:   sel,
	}
	m.mapStr = m.buildMap()
}

// openEventDetailPopup is one event's page: what happened, what it
// changed, and the thread of causes behind it — each cause one more
// page, so the chronicle browses as a web.
func (m *model) openEventDetailPopup(idx int) {
	if m.sim == nil || idx < 0 || idx >= len(m.sim.Log) {
		return
	}
	m.lastEventIdx = idx
	e := m.sim.Log[idx]
	body := []string{statusStyle.Render(e.Text)}
	if e.Detail != "" {
		body = append(body, dimStyle.Render(e.Detail))
	}
	opts := []popupOption{{label: "Jump there", action: popJumpXY}}
	if e.Cause >= 0 && e.Cause < len(m.sim.Log) {
		c := m.sim.Log[e.Cause]
		body = append(body, "", dimStyle.Render(fmt.Sprintf("grown from y%d — %s", c.Year, c.Text)))
		opts = append(opts, popupOption{
			label:  fmt.Sprintf("Follow the thread back (y%d)", c.Year),
			action: popEventDetail,
			arg:    e.Cause,
		})
	}
	opts = append(opts, popupOption{label: "Back to chronicle", action: popChronicle})
	m.popup = &popupState{
		title: fmt.Sprintf("year %d — %s", e.Year, e.Kind),
		body:  body,
		opts:  opts,
		cellX: e.X, cellY: e.Y,
	}
	m.mapStr = m.buildMap()
}

// jumpLatestNews moves the cursor to the most recent headline.
func (m *model) jumpLatestNews() {
	if !m.simMode || m.sim == nil {
		m.status = "no simulation running (S enters one)"
		return
	}
	for i := len(m.sim.Log) - 1; i >= 0; i-- {
		if e := m.sim.Log[i]; e.Major {
			m.curX, m.curY = e.X, e.Y
			m.status = fmt.Sprintf("y%d ⚑ %s", e.Year, e.Text)
			m.mapStr = m.buildMap()
			return
		}
	}
	m.status = "no news yet in this slice"
}
