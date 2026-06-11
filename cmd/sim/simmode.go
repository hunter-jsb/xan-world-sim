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
// index-match for the header readout.
var (
	simSpeeds     = []time.Duration{600 * time.Millisecond, 300 * time.Millisecond, 150 * time.Millisecond, 75 * time.Millisecond}
	simSpeedNames = []string{"½×", "1×", "2×", "4×"}
)

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
	m.simRuinCount = -1 // force the first features build
}

// simTickCmd schedules the next simulated year at the current speed.
func (m *model) simTickCmd() tea.Cmd {
	gen := m.simGen
	return tea.Tick(simSpeeds[m.simSpeed], func(time.Time) tea.Msg { return simTickMsg{gen: gen} })
}

// handleSimTick advances one year unless something holds the clock —
// an open popup (read in peace), an expedition afield (the court holds
// its breath, the day clock runs instead), or an explicit pause. The
// tick chain itself always continues while the mode is active.
func (m *model) handleSimTick(msg simTickMsg) tea.Cmd {
	if !m.simMode || msg.gen != m.simGen {
		return nil // stale tick from a left simulation
	}
	if m.popup == nil && m.exp == nil && !m.simPaused && m.sim != nil {
		events := m.sim.StepYear()
		territoryChanged := false
		var majors []world.SimEvent
		minor := ""
		for _, e := range events {
			switch e.Kind {
			case "secede", "swear", "dissolve", "ruin", "founding", "capture":
				territoryChanged = true
			}
			if e.Major {
				majors = append(majors, e)
			} else {
				minor = e.Text
			}
		}
		m.applySimData(territoryChanged)
		if minor != "" {
			m.status = fmt.Sprintf("year %d — %s", m.sim.Year, minor)
		} else {
			m.status = fmt.Sprintf("year %d", m.sim.Year)
		}
		if len(majors) > 0 {
			m.openMajorEventPopup(majors)
		}
		m.mapStr = m.buildMap()
	}
	return m.simTickCmd()
}

// applySimData overlays the simulation's political state onto the
// render data, in the same row shapes the DB queries produce. Seats
// change every year (allegiance, pressure); territory only when realm
// membership moved, so the grid rebuild is gated on that.
func (m *model) applySimData(territoryChanged bool) {
	if m.sim == nil {
		return
	}
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
			row := db.GetTerritoryInBoundsRow{X: tc.X, Y: tc.Y, RealmID: tc.RealmID}
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

// openMajorEventPopup interrupts for the year's major events — realm
// membership shifts and the epoch mark. The clock holds while it's up.
func (m *model) openMajorEventPopup(majors []world.SimEvent) {
	e := majors[0]
	body := make([]string, len(majors))
	for i, ev := range majors {
		body[i] = statusStyle.Render(ev.Text)
	}
	m.popup = &popupState{
		title: fmt.Sprintf("year %d — word reaches the halls", e.Year),
		body:  body,
		opts: []popupOption{
			{label: "Jump there", action: popJumpXY},
			{label: "Continue", action: popClose},
		},
		cellX: e.X, cellY: e.Y,
	}
}

// openChroniclePopup lists the slice's whole history, latest first;
// choosing an entry jumps the cursor to where it happened.
func (m *model) openChroniclePopup() {
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
		opts = append(opts, popupOption{
			label:  fmt.Sprintf("y%4d  %s", e.Year, e.Text),
			action: popJumpEvent,
			arg:    i,
		})
	}
	m.popup = &popupState{
		title: fmt.Sprintf("chronicle — year %d, %d entries", m.sim.Year, len(log)),
		opts:  opts,
		sel:   0,
	}
	m.mapStr = m.buildMap()
}
