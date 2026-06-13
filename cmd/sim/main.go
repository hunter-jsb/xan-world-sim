package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/migrations"
	"github.com/hunterjsb/xan-world-sim/internal/pqueue"
	"github.com/hunterjsb/xan-world-sim/internal/render"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

const (
	stepSmall = 5
	stepBig   = 25
)

func clampKya(k int) int {
	if k < 0 {
		return 0
	}
	if k > world.KyaMax {
		return world.KyaMax
	}
	return k
}

type model struct {
	ctx  context.Context
	conn *sql.DB
	q    *db.Queries

	// regenMu serializes regen Cmds. Bubble Tea fires Cmds in
	// goroutines, so spamming the kya keys can launch multiple
	// concurrent Generate→Persist→Query pipelines that race on
	// SQLite (which only allows one writer at a time and surfaces
	// the contention as "database is locked").
	regenMu *sync.Mutex

	// Raw world data — updated on each regen, used for instant
	// cursor re-renders without re-querying the DB.
	data worldData

	// Lookup maps built from raw data for O(1) cursor inspection.
	cellAt      map[[2]int64]db.GetCellsInBoundsRow
	riverAt     map[[2]int64]string // coord → river name
	seatAt      map[[2]int64]db.GetSeatsInBoundsRow
	featureAt   map[[2]int64]db.GetNamedFeaturesInBoundsRow
	territoryAt map[[2]int64]db.GetTerritoryInBoundsRow

	// lens selects the map's coloring: terrain, political, climate,
	// geological, ecological. Glyphs never change — a lens recolors
	// the world, it doesn't redraw it.
	lens int

	// popup is the active modal (nil = none). While open it captures
	// all input; options dispatch by action ID in handlePopupChoice.
	popup *popupState

	// pois is every named place in (y, x) hop order — w/b targets.
	pois []poi

	// Help-tree navigation: the folder path currently open, the last
	// root entry visited (the root menu reopens there), and the entry
	// index of the page being read (Back selects it).
	helpPath     []int
	helpSel      int
	helpEntrySel int

	// Streaming sim news: color pings at recent event sites (sim-year
	// memory), wall-clock event tags (the reading layer), the sticky
	// headline on the status line, and the last chronicle page read
	// (the chronicle reopens there).
	simPings        []simPing
	simTags         []simTag
	simNote         string
	simNoteMajor    bool
	simNoteDeadline time.Time
	lastEventIdx    int

	// Toast: a transient corner notice (overlays.go); gen guards the
	// fade timer against replacement by a newer toast.
	toastText string
	toastGen  int

	gridBuf *render.GridBuf // pre-rendered grid; Render() is fast on cursor moves
	mapStr  string
	legend  string
	seed    int64
	kya     int
	era     world.Era
	status  string

	// chain is the seed's sealed ages (fate.go) — folded into every
	// regen and every slice, persisted per seed, grown by sealing a
	// finished slice into the next step of deep time.
	chain []world.Fate

	// Cursor position on the map.
	curX, curY int64

	// exp is the active expedition (nil = none). expGen invalidates
	// in-flight tick Cmds after an abandon — a stale tick carrying an
	// old generation is discarded. dangerMap is pre-built per regen
	// from lair features and feeds route costs.
	exp       *expedition
	expGen    int
	dangerMap map[[2]int64]int
	dangerSrc map[[2]int64]string // dominant threat kind per dangerous cell

	// Simulation mode (simmode.go): the current kya pinned as a slice
	// of deep time, with the political layer running year by year.
	// simGen invalidates stale year ticks. preSim* stash the deep-time
	// state the sim overlays — seats and territory rows, and the view
	// mode — restored verbatim on exit (the sim never touches the DB).
	simMode         bool
	sim             *world.Sim
	simGen          int
	simPaused       bool
	simSpeed        int // index into simSpeeds
	preSimSeats     []db.GetSeatsInBoundsRow
	preSimTerritory []db.GetTerritoryInBoundsRow
	preSimCells     []db.GetCellsInBoundsRow
	preSimFeatures  []db.GetNamedFeaturesInBoundsRow
	preSimLens      int

	// Rise-and-fall bookkeeping: how many of the sim's cell patches
	// are already in m.data.cells, the ruin count the features list
	// was last built against, and the engine's territory version the
	// territory rows were last built from.
	simPatchesApplied int
	simRuinCount      int
	simTerrVersion    int

	minX, minY, maxX, maxY int64
}

// expPhase is the expedition lifecycle: proposed and waiting for the
// player's confirmation, marching on the day clock, or arrived.
type expPhase int

const (
	expPending expPhase = iota
	expRunning
	expArrived
)

// expedition is a caravan en route from the settlement nearest the
// chosen destination. Travel runs on a granular day clock inside one
// frozen moment of deep time: entering a cell costs its travel cost
// in *days* (river 1, open land 4, marsh 8, ...), so arrive[i] is the
// day the caravan reaches path[i].
type expedition struct {
	phase    expPhase
	fromName string
	destX    int64
	destY    int64
	path     []render.PathCell
	arrive   []int
	pos      int // index into path of the caravan's current cell
	day      int

	// En-route hazards (expevents.go): precomputed at departure,
	// fired in order on the day clock. delay is the days the road's
	// events have cost (or won); pending is the hazard awaiting the
	// player's choice; note keeps the last outcome on the status line
	// until day noteUntil so a tick doesn't immediately overwrite it.
	events    []expEvent
	nextEvent int
	delay     int
	pending   *expEvent
	note      string
	noteUntil int
}

func (e *expedition) totalDays() int { return e.arrive[len(e.arrive)-1] + e.delay }

// expTickMsg advances the day clock; gen guards against stale ticks.
type expTickMsg struct{ gen int }

// dayTick is the wall-clock pace of one expedition day.
const dayTick = 150 * time.Millisecond

// popupAction identifies what an option does when chosen — dispatched
// centrally in handlePopupChoice so popups stay declarative data.
type popupAction string

const (
	popClose       popupAction = "close"
	popSendExp     popupAction = "send-expedition" // from a cell dossier
	popDepart      popupAction = "depart"          // confirm a proposed expedition
	popCancelExp   popupAction = "cancel-expedition"
	popConclude    popupAction = "conclude-expedition"
	popJumpPOI     popupAction = "jump-poi"     // arg = index into m.pois
	popExitSim     popupAction = "exit-sim"     // leave simulation mode
	popSealAge     popupAction = "seal-age"     // distill the slice's fate, step deep time forward
	popJumpXY      popupAction = "jump-xy"      // jump cursor to the popup's cell
	popEventDetail popupAction = "event-detail" // arg = index into m.sim.Log — opens the event's page
	popChronicle   popupAction = "chronicle"    // back to the chronicle list
	popExpChoice   popupAction = "exp-choice"   // arg = index into the pending hazard's choices
	popHelpTopic   popupAction = "help-topic"   // arg = child index in the current help menu
	popHelpMenu    popupAction = "help-menu"    // back from a page to its menu
	popHelpUp      popupAction = "help-up"      // up one folder level
)

type popupOption struct {
	label  string
	action popupAction
	arg    int // action payload (e.g., POI index); 0 when unused
}

// popupState is one modal: a title, pre-styled body lines, and an
// optional action list. cellX/cellY carry the map context the popup
// was opened on (e.g., the dossier cell an expedition should target).
type popupState struct {
	title        string
	body         []string
	opts         []popupOption
	sel          int
	cellX, cellY int64
}

// worldData bundles the viewport query results the TUI renders from.
type worldData struct {
	cells     []db.GetCellsInBoundsRow
	rivers    []db.GetRiverCellsInBoundsRow
	roads     []db.GetRoadCellsInBoundsRow
	seats     []db.GetSeatsInBoundsRow
	features  []db.GetNamedFeaturesInBoundsRow
	territory []db.GetTerritoryInBoundsRow
}

// fetchWorldData loads everything in the viewport in one place — used
// both at startup and after each regen.
func fetchWorldData(ctx context.Context, q *db.Queries, minX, minY, maxX, maxY int64) (worldData, error) {
	var d worldData
	var err error
	d.cells, err = q.GetCellsInBounds(ctx, db.GetCellsInBoundsParams{
		X: minX, X_2: maxX, Y: minY, Y_2: maxY,
	})
	if err != nil {
		return d, fmt.Errorf("cells: %w", err)
	}
	d.rivers, err = q.GetRiverCellsInBounds(ctx, db.GetRiverCellsInBoundsParams{
		X: minX, X_2: maxX, Y: minY, Y_2: maxY,
	})
	if err != nil {
		return d, fmt.Errorf("rivers: %w", err)
	}
	d.roads, err = q.GetRoadCellsInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		return d, fmt.Errorf("roads: %w", err)
	}
	d.seats, err = q.GetSeatsInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		return d, fmt.Errorf("seats: %w", err)
	}
	d.features, err = q.GetNamedFeaturesInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		return d, fmt.Errorf("features: %w", err)
	}
	d.territory, err = q.GetTerritoryInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		return d, fmt.Errorf("territory: %w", err)
	}
	return d, nil
}

// regenMsg carries the raw query results from a regen Cmd. Rendering
// is deferred to the Update handler so cursor movements can re-render
// from stored data without re-querying the DB.
type regenMsg struct {
	data worldData
	seed int64
	kya  int
	era  world.Era
	err  error
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Modal popup captures all input while open (ctrl+c still quits).
		if m.popup != nil {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "q":
				return m.dismissPopup()
			case "up", "k":
				if len(m.popup.opts) > 0 {
					m.popup.sel = (m.popup.sel + len(m.popup.opts) - 1) % len(m.popup.opts)
					m.mapStr = m.buildMap()
				}
			case "down", "j":
				if len(m.popup.opts) > 0 {
					m.popup.sel = (m.popup.sel + 1) % len(m.popup.opts)
					m.mapStr = m.buildMap()
				}
			// Space deliberately doesn't choose: in sim mode a major
			// event popup can open the frame before a reflexive
			// space-to-pause, and choosing "Jump there" by accident
			// teleports the cursor. Enter is the advertised key.
			case "enter":
				if len(m.popup.opts) == 0 {
					return m.dismissPopup()
				}
				return m.handlePopupChoice(m.popup.opts[m.popup.sel])
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.exp != nil {
				m.abandonExpedition()
				return m, nil
			}
			if m.simMode {
				m.openSimExitPopup()
				return m, nil
			}
			return m, tea.Quit

		// Simulation mode: S pins the current kya as a slice of deep
		// time and lets politics run year by year; pressing it again
		// (or esc) asks before returning to deep time.
		case "S":
			if m.simMode {
				m.openSimExitPopup()
				return m, nil
			}
			return m, m.enterSimCmd()
		case " ":
			if m.simMode {
				m.simPaused = !m.simPaused
				if m.simPaused {
					return m, m.showToast("the years hold — space resumes")
				}
				return m, m.showToast("the years resume")
			}
		case "L":
			m.openChroniclePopup(-1)
		case "g":
			m.jumpLatestNews()
		case "enter":
			m.openCellPopup()
		case "r":
			if m.deepTimeLocked() {
				return m, nil
			}
			newSeed := time.Now().UnixNano()
			m.seed = newSeed
			// Every seed keeps its own sealed ages; a fresh roll almost
			// always starts with none.
			if chain, err := world.LoadFateChain(m.ctx, m.conn, newSeed); err == nil {
				m.chain = chain
			} else {
				m.chain = nil
			}
			m.status = "rerolling..."
			return m, m.regen(newSeed, m.kya)
		case "e":
			if m.deepTimeLocked() {
				return m, nil
			}
			next := world.KyaNow
			if m.kya == world.KyaNow {
				next = world.KyaOldWorld
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("jumping to %dkya...", next)
			return m, m.regen(m.seed, next)

		// The brackets drive time in both modes. Deep time: scrub kya
		// (`]` forward toward present, `[` backward toward the LGM,
		// braces take big steps). Inside a slice the world is pinned —
		// scrubbing would dissolve it under the sim — so the same keys
		// throttle the year clock instead: `]`/`[` step the speed,
		// `}`/`{` snap to the ends of the ladder.
		case "]":
			if m.simMode {
				return m, m.showToast(m.adjustSimSpeed(1))
			}
			if m.deepTimeLocked() {
				return m, nil
			}
			next := clampKya(m.kya - stepSmall)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "[":
			if m.simMode {
				return m, m.showToast(m.adjustSimSpeed(-1))
			}
			if m.deepTimeLocked() {
				return m, nil
			}
			next := clampKya(m.kya + stepSmall)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (past cap)", world.KyaMax)
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("← %dkya", next)
			return m, m.regen(m.seed, next)
		case "}", "shift+right":
			if m.simMode {
				return m, m.showToast(m.setSimSpeed(len(simSpeeds) - 1))
			}
			if m.deepTimeLocked() {
				return m, nil
			}
			next := clampKya(m.kya - stepBig)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("→→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "{", "shift+left":
			if m.simMode {
				return m, m.showToast(m.setSimSpeed(0))
			}
			if m.deepTimeLocked() {
				return m, nil
			}
			next := clampKya(m.kya + stepBig)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (past cap)", world.KyaMax)
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("←← %dkya", next)
			return m, m.regen(m.seed, next)

		// POI navigation: vim-style word-hop across named places.
		case "w":
			m.jumpPOI(1)
		case "b":
			m.jumpPOI(-1)
		case "o":
			m.openPOIPopup()

		// Help: glyph legend + key reference, in a popup.
		case "H":
			m.openHelpPopup()

		// Lenses: p cycles the map's coloring — terrain, political,
		// climate, geological, ecological — in both modes.
		case "p":
			m.lens = (m.lens + 1) % len(lensNames)
			m.rebuildGrid()
			return m, m.showToast("lens: " + lensNames[m.lens])

		// Expedition: s proposes a journey from the nearest settlement
		// to the cursor (confirm popup); s abandons one in progress.
		case "s":
			if m.exp == nil {
				m.planExpedition(m.curX, m.curY)
			} else {
				m.abandonExpedition()
			}

		// Cursor navigation — hjkl or arrows, instant (no regen needed).
		case "h", "left":
			if m.curX > m.minX {
				m.curX--
				m.mapStr = m.buildMap()
			}
		case "l", "right":
			if m.curX < m.maxX {
				m.curX++
				m.mapStr = m.buildMap()
			}
		case "k", "up":
			if m.curY > m.minY {
				m.curY--
				m.mapStr = m.buildMap()
			}
		case "j", "down":
			if m.curY < m.maxY {
				m.curY++
				m.mapStr = m.buildMap()
			}
		}

	case toastMsg:
		if msg.gen == m.toastGen && m.toastText != "" {
			m.toastText = ""
			m.mapStr = m.buildMap()
		}
		return m, nil

	case simTickMsg:
		return m, m.handleSimTick(msg)

	case simReadyMsg:
		// Discard a sim built for a moment the user scrubbed away from
		// (or if one is somehow already running).
		if msg.gen != m.simGen || msg.seed != m.seed || msg.kya != m.kya || m.simMode {
			return m, nil
		}
		return m, m.startSim(msg.sim)

	case expTickMsg:
		if m.exp == nil || m.exp.phase != expRunning || msg.gen != m.expGen {
			return m, nil // stale tick from an abandoned expedition
		}
		if m.popup != nil {
			// The day holds under any modal — a hazard choice, a cell
			// dossier, the chronicle — and resumes when it closes.
			return m, m.expTickCmd()
		}
		e := m.exp
		e.day++
		// The road's next hazard fires the day the caravan reaches it.
		if e.nextEvent < len(e.events) && e.day >= e.events[e.nextEvent].day+e.delay {
			ev := &e.events[e.nextEvent]
			e.nextEvent++
			e.pending = ev
			m.openExpEventPopup(ev)
			m.mapStr = m.buildMap()
			return m, m.expTickCmd()
		}
		for e.pos < len(e.path)-1 && e.arrive[e.pos+1]+e.delay <= e.day {
			e.pos++
		}
		if e.pos == len(e.path)-1 {
			e.phase = expArrived
			m.status = fmt.Sprintf("the %s expedition arrived at (%d,%d) after %d days",
				e.fromName, e.destX, e.destY, e.totalDays())
			m.popup = &popupState{
				title: "expedition arrived",
				body: []string{
					fmt.Sprintf("The %s expedition reached (%d,%d)", e.fromName, e.destX, e.destY),
					fmt.Sprintf("after %d days on the road.", e.totalDays()),
				},
				opts:  []popupOption{{label: "Conclude the expedition", action: popConclude}},
				cellX: e.destX, cellY: e.destY,
			}
			m.mapStr = m.buildMap()
			return m, nil
		}
		if e.note != "" && e.day < e.noteUntil {
			m.status = fmt.Sprintf("day %d/%d — %s", e.day, e.totalDays(), e.note)
		} else {
			m.status = fmt.Sprintf("day %d/%d — %s expedition afield",
				e.day, e.totalDays(), e.fromName)
		}
		m.mapStr = m.buildMap()
		return m, m.expTickCmd()

	case regenMsg:
		if msg.err != nil {
			m.status = "regen error: " + msg.err.Error()
			return m, nil
		}
		// Discard stale renders — user may have pressed more keys while
		// this Cmd was running, advancing m.kya/m.seed past this target.
		if msg.kya != m.kya || msg.seed != m.seed {
			return m, nil
		}
		m.data = msg.data
		m.buildLookups()
		m.rebuildGrid()
		if m.simMode {
			// A regen launched before the slice was pinned (an
			// in-flight scrub) landed mid-simulation. It's for this
			// same (seed, kya) — the staleness check above proved it —
			// so refresh the deep-time stash and re-overlay the sim's
			// politics (all cell patches replay); expeditions and
			// popups stay valid.
			m.stashDeepTime()
			m.applySimData(true)
		} else {
			// World changed — terrain costs shift, so a stale expedition
			// (or a popup about the old world) is meaningless now.
			m.exp = nil
			m.expGen++
			m.popup = nil
		}
		m.mapStr = m.buildMap()
		m.era = msg.era
		m.status = ""
		return m, nil
	}
	return m, nil
}

// buildLookups rebuilds the O(1) coord→data maps from the stored slices.
func (m *model) buildLookups() {
	m.cellAt = make(map[[2]int64]db.GetCellsInBoundsRow, len(m.data.cells))
	for _, c := range m.data.cells {
		m.cellAt[[2]int64{c.X, c.Y}] = c
	}
	m.riverAt = make(map[[2]int64]string, len(m.data.rivers))
	for _, r := range m.data.rivers {
		m.riverAt[[2]int64{r.X, r.Y}] = r.RiverName
	}
	m.seatAt = make(map[[2]int64]db.GetSeatsInBoundsRow, len(m.data.seats))
	for _, s := range m.data.seats {
		m.seatAt[[2]int64{s.X, s.Y}] = s
	}
	m.featureAt = make(map[[2]int64]db.GetNamedFeaturesInBoundsRow, len(m.data.features))
	for _, f := range m.data.features {
		m.featureAt[[2]int64{f.X, f.Y}] = f
	}
	m.territoryAt = make(map[[2]int64]db.GetTerritoryInBoundsRow, len(m.data.territory))
	for _, t := range m.data.territory {
		m.territoryAt[[2]int64{t.X, t.Y}] = t
	}
	var activityAt, heatAt func(x, y int64) float64
	if m.simMode && m.sim != nil {
		activityAt = m.sim.LairActivity
		heatAt = m.sim.VolcanoHeat
	}
	m.dangerMap, m.dangerSrc = buildDangerMap(m.data.features, activityAt, heatAt)
	m.pois = buildPOIs(m.data)
}

// rebuildGrid constructs the grid through the active lens.
func (m *model) rebuildGrid() {
	d := m.data
	switch m.lens {
	case lensPolitical:
		m.gridBuf = render.BuildPoliticalGridBuf(d.cells, d.rivers, d.roads, d.territory, m.minX, m.minY, m.maxX, m.maxY)
	case lensClimate:
		climate := world.ClimateAt(m.kya)
		m.gridBuf = render.BuildClimateGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY,
			func(x, y int64, elev float64) float64 {
				return world.Temperature(world.Latitude(int(y), world.DefaultLatTop, world.DefaultLatBottom), elev, climate)
			})
	case lensGeo:
		m.gridBuf = render.BuildGeoGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY,
			func(x, y int64) (int64, int64) {
				c := m.cellAt[[2]int64{x, y}]
				return c.Rock, c.RockAge
			})
	case lensEco:
		m.gridBuf = render.BuildEcoGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY)
	case lensHydro:
		m.gridBuf = render.BuildHydroGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY,
			func(x, y int64) int64 { return m.cellAt[[2]int64{x, y}].Drainage })
	case lensDanger:
		m.gridBuf = render.BuildDangerGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY,
			func(x, y int64) int { return m.dangerMap[[2]int64{x, y}] })
	default:
		m.gridBuf = render.BuildGridBuf(d.cells, d.rivers, d.roads, m.minX, m.minY, m.maxX, m.maxY)
	}
}

// buildMap renders the grid using the cached GridBuf — only the cursor
// row is re-rendered, so cursor moves are ~50× faster than a full regen.
func (m *model) buildMap() string {
	if m.gridBuf == nil {
		return ""
	}
	if m.popup != nil {
		box := render.PopupBox(m.popup.title, m.popup.body, popupLabels(m.popup.opts), m.popup.sel)
		return m.gridBuf.RenderWithOverlay(m.curX, m.curY, m.overlayPath(), box)
	}
	if overlays := m.mapOverlays(); len(overlays) > 0 {
		return m.gridBuf.RenderWithCallouts(m.curX, m.curY, m.overlayPath(), overlays)
	}
	return m.gridBuf.Render(m.curX, m.curY, m.overlayPath())
}

// cellInfoAt assembles a CellInfo for the cursor position.
func (m *model) cellInfoAt(x, y int64) render.CellInfo {
	info := render.CellInfo{X: x, Y: y}
	if c, ok := m.cellAt[[2]int64{x, y}]; ok {
		info.Kind = c.Kind
		info.Elev = c.Elevation
		if rock := world.RockKind(c.Rock); rock != "" {
			switch {
			case c.RockAge <= 0:
				rock += ", fresh"
			case c.RockAge < 250:
				rock += fmt.Sprintf(", laid %d ka ago", c.RockAge)
			}
			info.RockNote = rock
		}
	}
	if rn, ok := m.riverAt[[2]int64{x, y}]; ok {
		info.RiverName = rn
	}
	if s, ok := m.seatAt[[2]int64{x, y}]; ok {
		info.SeatName = s.Name
		info.SeatPressure = s.Pressure
		if s.RealmName != "" {
			info.RealmID = s.RealmID
			info.RealmName = s.RealmName
			info.RealmIsCrown = s.IsCrown
			info.SeatStance = fmt.Sprintf("%s (%.2f)",
				world.AllegianceStance(s.Allegiance), s.Allegiance)
		}
	} else if t, ok := m.territoryAt[[2]int64{x, y}]; ok {
		info.RealmID = t.RealmID
		info.RealmName = t.RealmName
		info.RealmIsCrown = t.IsCrown
	}
	if f, ok := m.featureAt[[2]int64{x, y}]; ok {
		info.FeatureName = f.Name
		info.FeatureDetail = f.Detail
	}
	return info
}

func (m model) regen(seed int64, kya int) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				simLog("panic seed=%d kya=%d: %v\n%s", seed, kya, r, debug.Stack())
				msg = regenMsg{err: fmt.Errorf("panic at kya=%d: %v", kya, r)}
			}
		}()
		m.regenMu.Lock()
		defer m.regenMu.Unlock()
		simLog("regen seed=%d kya=%d fates=%d", seed, kya, len(m.chain))
		w := world.GenerateWithFates(seed, kya, m.chain)
		if err := world.Persist(m.ctx, m.conn, w); err != nil {
			simLog("persist failed seed=%d kya=%d: %v", seed, kya, err)
			return regenMsg{err: fmt.Errorf("persist: %w", err)}
		}
		data, err := fetchWorldData(m.ctx, m.q, m.minX, m.minY, m.maxX, m.maxY)
		if err != nil {
			return regenMsg{err: err}
		}
		simLog("ok seed=%d kya=%d cells=%d rivers=%d roads=%d seats=%d features=%d",
			seed, kya, len(data.cells), len(data.rivers), len(data.roads), len(data.seats), len(data.features))
		return regenMsg{data: data, seed: seed, kya: kya, era: w.Era}
	}
}

// simLog appends a timestamped line to xan-sim.log in the OS temp
// directory (/tmp on Linux, %TMP% on Windows).
func simLog(format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "xan-sim.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format("15:04:05.000")}, args...)...)
}

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	seedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Italic(true)
)

func (m model) View() string {
	var b strings.Builder
	title := render.Title("xan-world-sim — the cradle")
	title += dimStyle.Render("   t: ") + seedStyle.Render(fmt.Sprintf("%dkya", m.kya))
	if string(m.era) != "" && string(m.era) != fmt.Sprintf("%dkya", m.kya) {
		title += dimStyle.Render(" (") + seedStyle.Render(string(m.era)) + dimStyle.Render(")")
	}
	gI := world.GlacialIndex(m.kya)
	title += dimStyle.Render("   glacial: ") + seedStyle.Render(fmt.Sprintf("%.2f", gI))
	if n := len(m.chain); n > 0 {
		title += dimStyle.Render("   ages sealed: ") + seedStyle.Render(fmt.Sprintf("%d", n))
	}
	if m.lens != lensTerrain {
		title += dimStyle.Render("   [") + statusStyle.Render(lensNames[m.lens]) + dimStyle.Render("]")
	}
	if m.simMode && m.sim != nil {
		run := "▸"
		if m.simPaused {
			run = "‖"
		}
		clock := fmt.Sprintf("year %d", m.sim.Year)
		if simSpeeds[m.simSpeed].months < monthsPerYearUI {
			clock = fmt.Sprintf("year %d m%02d", m.sim.Year, m.sim.Month())
		}
		title += dimStyle.Render("   sim: ") + seedStyle.Render(fmt.Sprintf("%s %s%s", clock, run, simSpeeds[m.simSpeed].name))
	}
	if m.seed != 0 {
		title += dimStyle.Render("   seed: ") + seedStyle.Render(fmt.Sprintf("%d", m.seed))
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(m.mapStr)
	b.WriteString("\n\n")
	b.WriteString(render.InfoPanel(m.cellInfoAt(m.curX, m.curY)))
	b.WriteString("\n\n")
	// Footer: only what matters right now — popup navigation when one
	// is open, modal expedition prompts, the help key, and the status
	// line. The full reference lives behind H.
	var hints []string
	switch {
	case m.popup != nil:
		hints = append(hints, "↑↓ select   enter choose   esc close")
	default:
		switch {
		case m.exp != nil:
			switch m.exp.phase {
			case expRunning:
				hints = append(hints, fmt.Sprintf("s abandon (day %d/%d)", m.exp.day, m.exp.totalDays()))
			case expArrived:
				hints = append(hints, "s conclude expedition")
			}
		case m.simMode:
			pause := "space pause"
			if m.simPaused {
				pause = "space resume"
			}
			hints = append(hints, pause, "] [ speed", "L chronicle", "g news", "S leave")
		default:
			hints = append(hints, "enter inspect")
		}
		hints = append(hints, "H help", "q quit")
	}
	b.WriteString(dimStyle.Render(strings.Join(hints, "   ")))
	if m.status != "" {
		b.WriteString("   ")
		b.WriteString(statusStyle.Render(m.status))
	}
	return b.String()
}

// popupLabels extracts option labels for the render layer.
func popupLabels(opts []popupOption) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.label
	}
	return out
}

func main() {
	printOnce := flag.Bool("print", false, "render map once to stdout and exit (no TUI)")
	flag.Parse()

	dbPath := "world.db"
	if v := os.Getenv("XAN_DB"); v != "" {
		dbPath = v
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		log.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	q := db.New(conn)
	ctx := context.Background()

	const minX, minY = 0, 0
	maxX, maxY := int64(world.Width-1), int64(world.Height-1)

	seed := readMetaInt(ctx, conn, "seed")
	kya := int(readMetaInt(ctx, conn, "kya"))
	chain, err := world.LoadFateChain(ctx, conn, seed)
	if err != nil {
		log.Fatalf("load fate chain: %v", err)
	}
	if err := world.Persist(ctx, conn, world.GenerateWithFates(seed, kya, chain)); err != nil {
		log.Fatalf("bootstrap world: %v", err)
	}

	data, err := fetchWorldData(ctx, q, minX, minY, maxX, maxY)
	if err != nil {
		log.Fatalf("fetch world: %v", err)
	}

	era := world.EraForKya(kya)

	if *printOnce {
		fmt.Println(render.Title("xan-world-sim — the cradle"))
		fmt.Println()
		fmt.Println(render.Grid(data.cells, data.rivers, data.roads, minX, minY, maxX, maxY, -1, -1))
		fmt.Println()
		fmt.Println(render.Legend())
		fmt.Printf("\nt: %dkya (%s)   glacial: %.2f   seed: %d\n",
			kya, era, world.GlacialIndex(kya), seed)
		return
	}

	// Cursor starts at map center.
	initCurX := (minX + maxX) / 2
	initCurY := (minY + maxY) / 2

	m := model{
		ctx: ctx, conn: conn, q: q,
		regenMu: &sync.Mutex{},
		data:    data,
		legend:  render.Legend(),
		seed:    seed,
		chain:   chain,
		kya:     kya,
		era:     era,
		curX:    initCurX,
		curY:    initCurY,
		minX:    minX,
		minY:    minY,
		maxX:    maxX,
		maxY:    maxY,
		status:  "press H for the legend and keys",
	}
	m.buildLookups()
	m.rebuildGrid()
	m.mapStr = m.buildMap()

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("tea: %v", err)
	}
}

// expHeapItem is a Dijkstra priority queue entry.
type expHeapItem struct {
	x, y int64
	cost int
}

// deepTimeLocked reports whether kya scrubbing and rerolls are blocked
// — true whenever an expedition exists or a simulation is running.
// Both live inside one frozen moment of deep time; scrubbing would
// dissolve the world under them. Sets the status line as a side effect.
func (m *model) deepTimeLocked() bool {
	if m.exp != nil {
		m.status = "deep time is locked while an expedition is afield (s abandons)"
		return true
	}
	if m.simMode {
		m.status = "deep time is pinned while the simulation runs (S returns)"
		return true
	}
	return false
}

func (m *model) abandonExpedition() {
	if m.exp != nil && m.exp.phase == expArrived {
		m.status = "expedition concluded"
	} else {
		m.status = "expedition abandoned"
	}
	m.exp = nil
	m.expGen++ // invalidate in-flight ticks
	m.mapStr = m.buildMap()
}

// dismissPopup closes the modal. Closing a proposal popup cancels the
// pending expedition — esc means "no" to a confirm dialog.
func (m model) dismissPopup() (tea.Model, tea.Cmd) {
	m.popup = nil
	if m.exp != nil && m.exp.phase == expPending {
		m.exp = nil
		m.expGen++
		m.status = "expedition cancelled"
	}
	m.mapStr = m.buildMap()
	return m, nil
}

// handlePopupChoice dispatches the chosen option's action.
func (m model) handlePopupChoice(opt popupOption) (tea.Model, tea.Cmd) {
	pop := m.popup
	m.popup = nil
	switch opt.action {
	case popClose:
		// just closes

	case popJumpPOI:
		if opt.arg >= 0 && opt.arg < len(m.pois) {
			p := m.pois[opt.arg]
			m.curX, m.curY = p.X, p.Y
			m.status = fmt.Sprintf("→ %s (%s)", p.Name, render.KindDisplay(p.Kind))
		}

	case popExitSim:
		m.exitSim()
		m.mapStr = m.buildMap()
		return m, m.showToast("returned to deep time")

	case popSealAge:
		if m.sim == nil || !m.sim.EpochReached() || m.kya <= 0 {
			break
		}
		fate := world.DistillFate(m.sim)
		if err := world.SaveFate(m.ctx, m.conn, fate); err != nil {
			m.status = fmt.Sprintf("the age would not seal: %v", err)
			break
		}
		// Branch semantics in memory, mirroring SaveFate: this future
		// replaces any previously sealed at or after this moment.
		kept := m.chain[:0:0]
		for _, f := range m.chain {
			if f.Kya > fate.Kya {
				kept = append(kept, f)
			}
		}
		m.chain = append(kept, fate)
		m.exitSim()
		m.kya--
		m.era = world.EraForKya(m.kya)
		m.mapStr = m.buildMap()
		return m, tea.Batch(m.regen(m.seed, m.kya),
			m.showToast(fmt.Sprintf("the age is sealed — deep time steps to %dkya carrying its fate", m.kya)))

	case popJumpXY:
		m.curX, m.curY = pop.cellX, pop.cellY

	case popEventDetail:
		m.openEventDetailPopup(opt.arg)
	case popChronicle:
		m.openChroniclePopup(m.lastEventIdx)

	case popSendExp:
		m.mapStr = m.buildMap()
		m.planExpedition(pop.cellX, pop.cellY)
		return m, nil

	case popDepart:
		if m.exp != nil && m.exp.phase == expPending {
			m.exp.phase = expRunning
			m.status = fmt.Sprintf("the %s expedition departs — %d days ahead",
				m.exp.fromName, m.exp.totalDays())
			m.mapStr = m.buildMap()
			return m, m.expTickCmd()
		}

	case popCancelExp:
		if m.exp != nil && m.exp.phase == expPending {
			m.exp = nil
			m.expGen++
			m.status = "expedition cancelled"
		}

	case popConclude:
		m.exp = nil
		m.expGen++
		m.status = "expedition concluded"

	case popExpChoice:
		m.resolveExpChoice(opt.arg)

	case popHelpTopic:
		m.openHelpEntry(opt.arg)
	case popHelpMenu:
		m.openHelpMenu(m.helpEntrySel)
	case popHelpUp:
		m.helpUp()
	}
	m.mapStr = m.buildMap()
	return m, nil
}

// openCellPopup shows the full dossier for the cursor cell — the
// info panel's data plus everything that doesn't fit on one line —
// with context actions.
func (m *model) openCellPopup() {
	info := m.cellInfoAt(m.curX, m.curY)
	title := fmt.Sprintf("(%d,%d)", m.curX, m.curY)
	var body []string
	add := func(format string, args ...any) {
		body = append(body, dimStyle.Render(fmt.Sprintf(format, args...)))
	}
	if info.Kind == "" {
		add("uncharted void")
	} else {
		switch {
		case info.SeatName != "":
			title = info.SeatName
		case info.FeatureName != "":
			title = info.FeatureName
		case info.RiverName != "":
			title = "the " + info.RiverName
		}
		add("terrain: %s   elev %.0fm", info.Kind, info.Elev)
		if info.SeatName != "" {
			add("seat: %s", info.SeatName)
		}
		if info.RealmName != "" {
			line := "realm: " + info.RealmName
			if info.SeatStance != "" {
				line += "   " + info.SeatStance
			}
			add("%s", line)
		}
		// Heritage lines live only inside a running slice — deep time
		// doesn't track who reigns.
		if m.simMode && m.sim != nil {
			if m.sim.Contested(m.curX, m.curY) {
				add("contested marchland — two banners claim this ground")
			}
			if h, since := m.sim.HouseAt(m.curX, m.curY); h != "" {
				if since > 0 {
					add("house: House %s (took the hall in year %d)", h, since)
				} else {
					add("house: House %s (an old line)", h)
				}
			}
			if info.RealmID != 0 {
				if l := m.sim.RealmLineage(info.RealmID); l != "" {
					add("%s", l)
				}
				for _, w := range m.sim.Wars() {
					if w.A == info.RealmID || w.B == info.RealmID {
						other := w.A
						if other == info.RealmID {
							other = w.B
						}
						add("at war with %s (since year %d)", m.sim.RealmDisplayName(other), w.Start)
					}
				}
			}
		}
		if info.RiverName != "" {
			add("river: %s", info.RiverName)
		}
		if info.FeatureName != "" {
			line := "feature: " + info.FeatureName
			if info.FeatureDetail != "" {
				line += "   " + info.FeatureDetail
			}
			add("%s", line)
		}
		if info.SeatPressure > 0 {
			add("dragon pressure: %.0f", info.SeatPressure)
		}
		if d := m.dangerMap[[2]int64{m.curX, m.curY}]; d > 0 {
			add("lair danger: +%d days/cell", d)
		}
		if c := m.pathCellCost(m.curX, m.curY); c >= 0 {
			add("travel: %d days per cell", c)
		} else {
			add("travel: impassable")
		}
	}
	opts := []popupOption{}
	if m.exp == nil && m.pathCellCost(m.curX, m.curY) >= 0 {
		opts = append(opts, popupOption{label: "Send expedition here", action: popSendExp})
	}
	opts = append(opts, popupOption{label: "Close", action: popClose})
	m.popup = &popupState{
		title: title, body: body, opts: opts,
		cellX: m.curX, cellY: m.curY,
	}
	m.mapStr = m.buildMap()
}

// planExpedition proposes a journey: the settlement nearest the
// destination (by travel cost, dragon danger included — the world's
// own metric of "near") offers to send a caravan there. A confirm
// popup asks before anyone marches.
func (m *model) planExpedition(destX, destY int64) {
	if m.pathCellCost(destX, destY) < 0 {
		m.status = "no expedition can reach this terrain"
		return
	}
	seat, ok := m.nearestSeat(destX, destY)
	if !ok {
		m.status = "no settlement can reach this place"
		return
	}
	path := m.computePath(seat.X, seat.Y, destX, destY)
	if len(path) < 2 {
		m.status = "the destination is at the hall's own doorstep"
		return
	}
	// The origin renders as part of the trail, not as a second
	// caravan marker — the caravan glyph is drawn at pos.
	path[0].G = render.DirectionalGlyph(int(path[1].X-path[0].X), int(path[1].Y-path[0].Y))
	arrive := make([]int, len(path))
	for i := 1; i < len(path); i++ {
		arrive[i] = arrive[i-1] + m.pathCellCost(path[i].X, path[i].Y)
	}
	events := m.buildExpEvents(seat.RealmID, path, arrive)
	m.exp = &expedition{
		phase:    expPending,
		fromName: seat.Name,
		destX:    destX,
		destY:    destY,
		path:     path,
		arrive:   arrive,
		events:   events,
	}
	body := []string{
		dimStyle.Render(fmt.Sprintf("%s (%d,%d) offers a caravan to (%d,%d).",
			seat.Name, seat.X, seat.Y, destX, destY)),
		dimStyle.Render(fmt.Sprintf("the road: %d cells, %d days", len(path)-1, arrive[len(arrive)-1])),
	}
	if n := len(events); n == 1 {
		body = append(body, dimStyle.Render("the scouts mark one hazard on this road"))
	} else if n > 1 {
		body = append(body, dimStyle.Render(fmt.Sprintf("the scouts mark %d hazards on this road", n)))
	}
	body = append(body, dimStyle.Render("deep time stays locked until it returns or is abandoned"))
	m.popup = &popupState{
		title: "expedition",
		body:  body,
		opts:  []popupOption{{label: "Depart", action: popDepart}, {label: "Cancel", action: popCancelExp}},
		cellX: destX, cellY: destY,
	}
	m.status = ""
	m.mapStr = m.buildMap()
}

// nearestSeat finds the settlement with the cheapest route to (x, y)
// via Dijkstra outward from the destination over entry costs. Ties
// keep the first seat in query order (stable for a given world).
func (m *model) nearestSeat(x, y int64) (db.GetSeatsInBoundsRow, bool) {
	type coord = [2]int64
	dist := make(map[coord]int, 512)
	dist[coord{x, y}] = 0
	h := pqueue.New(func(a, b expHeapItem) bool { return a.cost < b.cost })
	h.Push(expHeapItem{x, y, 0})
	dirs := [8][2]int64{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}
	for h.Len() > 0 {
		cur := h.Pop()
		cc := coord{cur.x, cur.y}
		if cur.cost > dist[cc] {
			continue
		}
		for _, d := range dirs {
			nx, ny := cur.x+d[0], cur.y+d[1]
			if nx < m.minX || nx > m.maxX || ny < m.minY || ny > m.maxY {
				continue
			}
			c := m.pathCellCost(nx, ny)
			if c < 0 {
				continue
			}
			nc := coord{nx, ny}
			nd := dist[cc] + c
			if old, seen := dist[nc]; !seen || nd < old {
				dist[nc] = nd
				h.Push(expHeapItem{nx, ny, nd})
			}
		}
	}
	best := -1
	bestD := 0
	for i, s := range m.data.seats {
		d, ok := dist[coord{s.X, s.Y}]
		if !ok {
			continue
		}
		if best < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return db.GetSeatsInBoundsRow{}, false
	}
	return m.data.seats[best], true
}

// expTickCmd schedules the next expedition day.
func (m *model) expTickCmd() tea.Cmd {
	gen := m.expGen
	return tea.Tick(dayTick, func(time.Time) tea.Msg { return expTickMsg{gen: gen} })
}

// overlayPath renders the expedition route for the current phase
// (walked trail dimmed, caravan at pos, road ahead bright, X at the
// destination) plus the simulation's event pings — alarm-tinted
// cells where news recently broke (zero glyph = recolor only).
func (m *model) overlayPath() []render.PathCell {
	var out []render.PathCell
	if e := m.exp; e != nil {
		out = make([]render.PathCell, len(e.path), len(e.path)+len(m.simPings))
		copy(out, e.path)
		for i := range out {
			if i < e.pos {
				out[i].Dim = true
			}
		}
		if e.phase != expPending {
			out[e.pos].G = '@'
			out[e.pos].Dim = false
		}
	}
	for _, p := range m.simPings {
		out = append(out, render.PathCell{X: p.x, Y: p.y, Hot: true})
	}
	return out
}

// buildDangerMap pre-computes per-cell dragon danger scores from all lair
// features. Dens radiate danger within radius 12 (scale ×3), nests 8 (×2),
// rookeries 6 (×1). Overlapping zones take the maximum score.
//
// activityAt scales each lair's danger by its current raid activity
// (sim mode: routes get costlier under a rampant dragon and cheaper
// past a dormant one); nil means the static generation-time level.
// Volcanoes radiate through the same map (radius 8 ×3, scaled by
// heat — live in a slice via heatAt, else from the persisted
// last-eruption age): one danger field, every source. The returned
// source map names the dominant threat per cell so the expedition
// events can flavor the encounter honestly.
func buildDangerMap(features []db.GetNamedFeaturesInBoundsRow, activityAt, heatAt func(x, y int64) float64) (map[[2]int64]int, map[[2]int64]string) {
	danger := make(map[[2]int64]int)
	source := make(map[[2]int64]string)
	for _, f := range features {
		var radius, scale int
		switch f.Kind {
		case "den":
			radius, scale = 12, 3
		case "nest":
			radius, scale = 8, 2
		case "rookery":
			radius, scale = 6, 1
		case "volcano":
			radius, scale = 8, 3
		default:
			continue
		}
		activity := 1.0
		if f.Kind == "volcano" {
			if heatAt != nil {
				activity = heatAt(f.X, f.Y)
			} else {
				// Static heat from the persisted last-eruption age.
				activity = 1 - float64(f.Meta)/50
				if activity < 0 {
					activity = 0
				}
			}
		} else if activityAt != nil {
			activity = activityAt(f.X, f.Y)
		}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				cheb := int64(dx)
				if cheb < 0 {
					cheb = -cheb
				}
				if ady := int64(dy); ady < 0 {
					if -ady > cheb {
						cheb = -ady
					}
				} else if ady > cheb {
					cheb = ady
				}
				d := int(float64((radius-int(cheb))*scale)*activity + 0.5)
				if d <= 0 {
					continue
				}
				k := [2]int64{f.X + int64(dx), f.Y + int64(dy)}
				if d > danger[k] {
					danger[k] = d
					source[k] = f.Kind
				}
			}
		}
	}
	return danger, source
}

// pathCellCost returns the movement cost to enter (x, y), or -1 if impassable.
func (m *model) pathCellCost(x, y int64) int {
	coord := [2]int64{x, y}
	danger := m.dangerMap[coord]

	if m.riverAt[coord] != "" {
		return 1 + danger
	}

	c, ok := m.cellAt[coord]
	if !ok {
		return -1
	}
	base := world.TravelCost(c.Kind)
	if base < 0 {
		return -1
	}
	return base + danger
}

// computePath runs weighted Dijkstra from (sx,sy) to (ex,ey) and returns
// the path as PathCells with directional glyphs. Returns nil if unreachable.
func (m *model) computePath(sx, sy, ex, ey int64) []render.PathCell {
	type coord = [2]int64
	start := coord{sx, sy}
	end := coord{ex, ey}

	dist := make(map[coord]int, 512)
	prev := make(map[coord]coord, 512)
	dist[start] = 0

	h := pqueue.New(func(a, b expHeapItem) bool { return a.cost < b.cost })
	h.Push(expHeapItem{sx, sy, 0})

	dirs := [8][2]int64{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

	for h.Len() > 0 {
		cur := h.Pop()
		cc := coord{cur.x, cur.y}
		if cc == end {
			break
		}
		if cur.cost > dist[cc] {
			continue // stale entry
		}
		for _, d := range dirs {
			nx, ny := cur.x+d[0], cur.y+d[1]
			if nx < m.minX || nx > m.maxX || ny < m.minY || ny > m.maxY {
				continue
			}
			nc := coord{nx, ny}
			cost := m.pathCellCost(nx, ny)
			if cost < 0 {
				continue
			}
			newDist := dist[cc] + cost
			if d, seen := dist[nc]; !seen || newDist < d {
				dist[nc] = newDist
				prev[nc] = cc
				h.Push(expHeapItem{nx, ny, newDist})
			}
		}
	}

	if _, reached := dist[end]; !reached {
		return nil
	}

	var steps []coord
	for c := end; c != start; c = prev[c] {
		steps = append(steps, c)
	}
	steps = append(steps, start)
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}

	result := make([]render.PathCell, len(steps))
	for i, c := range steps {
		var g rune
		switch {
		case i == 0:
			g = '@'
		case i == len(steps)-1:
			g = 'X'
		default:
			next := steps[i+1]
			g = render.DirectionalGlyph(int(next[0]-c[0]), int(next[1]-c[1]))
		}
		result[i] = render.PathCell{X: c[0], Y: c[1], G: g}
	}
	return result
}

func readMetaString(ctx context.Context, conn *sql.DB, key string) string {
	var s string
	err := conn.QueryRowContext(ctx, "SELECT value FROM world_meta WHERE key = ?", key).Scan(&s)
	if err != nil {
		return ""
	}
	return s
}

func readMetaInt(ctx context.Context, conn *sql.DB, key string) int64 {
	s := readMetaString(ctx, conn, key)
	if s == "" {
		return 0
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}
