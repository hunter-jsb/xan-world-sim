package main

import (
	"fmt"
	"math/rand"

	"github.com/hunterjsb/xan-world-sim/internal/render"
)

// En-route expedition events. The road itself writes them: deep lair
// danger means a sighting, long marsh means fever, a long river leg
// means a swift current, and (mid-sim) territory of a realm at war
// with the caravan's home means riders demanding toll. Everything is
// precomputed at departure from the route, with outcomes pre-rolled
// from a journey-keyed seed — the same journey always meets the same
// fortune, and choosing late changes nothing but the choosing.

const (
	// dangerSightingMin is the danger-map score that turns a route
	// cell into dragon country worth an event — the deep core of den
	// and nest reach; wyvern fringes stay mere travel cost.
	dangerSightingMin = 15

	// marshFeverRun is how many consecutive marsh cells make a fever
	// camp; riverFortuneRun how many consecutive river cells make a
	// swift-current leg.
	marshFeverRun   = 3
	riverFortuneRun = 12
)

// expChoice is one option on an event popup, declaratively: flat day
// cost, an optional risk (resolved against the event's pre-rolled
// fortune), or turning back entirely.
type expChoice struct {
	label     string
	delay     int
	riskOdds  float64
	riskDelay int
	riskText  string
	safeText  string
	turnBack  bool
}

// expEvent is one hazard on the road, fixed at departure.
type expEvent struct {
	day     int // base day the caravan reaches the trigger cell
	x, y    int64
	title   string
	body    []string
	choices []expChoice
	roll    float64 // pre-rolled fortune in [0,1)
}

// expEventSeed keys a journey's fortune: same world, same road, same
// luck — the road is the road.
func expEventSeed(worldSeed int64, sx, sy, dx, dy int64) int64 {
	return worldSeed ^ (sx * 1000003) ^ (sy * 7919) ^ (dx * 31337) ^ (dy * 31)
}

// buildExpEvents scans the route and lays its hazards on the day
// clock. Zones the caravan starts inside don't fire — home ground is
// not an encounter.
func (m *model) buildExpEvents(fromRealm int64, path []render.PathCell, arrive []int) []expEvent {
	if len(path) == 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(expEventSeed(m.seed,
		path[0].X, path[0].Y, path[len(path)-1].X, path[len(path)-1].Y)))
	var events []expEvent

	inDanger := m.dangerMap[[2]int64{path[0].X, path[0].Y}] >= dangerSightingMin
	marshRun := 0
	if c, ok := m.cellAt[[2]int64{path[0].X, path[0].Y}]; ok && c.Kind == "marsh" {
		marshRun = marshFeverRun // already camped in marsh: no fresh fever event
	}
	riverRun := 0
	if m.riverAt[[2]int64{path[0].X, path[0].Y}] != "" {
		riverRun = riverFortuneRun
	}
	tolled := map[int64]bool{}
	if t, ok := m.territoryAt[[2]int64{path[0].X, path[0].Y}]; ok {
		tolled[t.RealmID] = true
	}

	for i := 1; i < len(path); i++ {
		p := [2]int64{path[i].X, path[i].Y}

		if d := m.dangerMap[p]; d >= dangerSightingMin {
			if !inDanger {
				inDanger = true
				mauled := 2 + d/6
				events = append(events, expEvent{
					day: arrive[i], x: p[0], y: p[1],
					title: "wings on the horizon",
					body: []string{
						dimStyle.Render(fmt.Sprintf("the road ahead runs under a lair's shadow (danger %d)", d)),
						dimStyle.Render("the drovers look to the sky, then to you"),
					},
					choices: []expChoice{
						{label: "Press on", riskOdds: 0.5, riskDelay: mauled,
							riskText: fmt.Sprintf("the beast finds the caravan — %d days lost tending the mauled", mauled),
							safeText: "the shadow passes over; the road goes on"},
						{label: "Shelter until it passes (+3 days)", delay: 3,
							safeText: "three days in a cold camp, but every soul accounted for"},
						{label: "Turn back", turnBack: true},
					},
					roll: rng.Float64(),
				})
			}
		} else {
			inDanger = false
		}

		if c, ok := m.cellAt[p]; ok && c.Kind == "marsh" {
			marshRun++
			if marshRun == marshFeverRun {
				events = append(events, expEvent{
					day: arrive[i], x: p[0], y: p[1],
					title: "fever in the camp",
					body: []string{
						dimStyle.Render("days of black water and biting flies — the first porters are shivering"),
					},
					choices: []expChoice{
						{label: "Rest and boil water (+4 days)", delay: 4,
							safeText: "the fever breaks in camp; the road waits"},
						{label: "March through", riskOdds: 0.5, riskDelay: 8,
							riskText: "the fever spreads on the march — 8 days lost",
							safeText: "the column outwalks the sickness"},
					},
					roll: rng.Float64(),
				})
			}
		} else {
			marshRun = 0
		}

		if m.riverAt[p] != "" {
			riverRun++
			if riverRun == riverFortuneRun {
				events = append(events, expEvent{
					day: arrive[i], x: p[0], y: p[1],
					title: "the current runs swift",
					body: []string{
						dimStyle.Render(fmt.Sprintf("the %s is high and fast — the boats could run it through the nights", m.riverAt[p])),
					},
					choices: []expChoice{
						{label: "Ride the current (−3 days)", delay: -3,
							safeText: "the river does the marching — 3 days won"},
						{label: "Put ashore each night",
							safeText: "slow and sure, ashore by dusk"},
					},
					roll: rng.Float64(),
				})
			}
		} else {
			riverRun = 0
		}

		// Tolls only exist mid-sim: deep time has no wars.
		if m.simMode && m.sim != nil && fromRealm != 0 {
			if t, ok := m.territoryAt[p]; ok && t.RealmID != 0 && !tolled[t.RealmID] {
				tolled[t.RealmID] = true
				if t.RealmID != fromRealm && m.sim.AtWar(t.RealmID, fromRealm) {
					events = append(events, expEvent{
						day: arrive[i], x: p[0], y: p[1],
						title: "riders bar the road",
						body: []string{
							dimStyle.Render(fmt.Sprintf("this is the land of %s, at war with the caravan's banner", t.RealmName)),
							dimStyle.Render("their riders demand a passage-price"),
						},
						choices: []expChoice{
							{label: "Pay the toll",
								safeText: "the chests are lighter, the road open"},
							{label: "Refuse and detour (+5 days)", delay: 5,
								safeText: "five days of goat tracks around their patrols"},
							{label: "Turn back", turnBack: true},
						},
						roll: rng.Float64(),
					})
				}
			}
		}
	}
	return events
}

// openExpEventPopup raises the pending hazard as a modal; the day
// clock holds while it's up.
func (m *model) openExpEventPopup(ev *expEvent) {
	opts := make([]popupOption, len(ev.choices))
	for i, c := range ev.choices {
		opts[i] = popupOption{label: c.label, action: popExpChoice, arg: i}
	}
	m.popup = &popupState{
		title: fmt.Sprintf("%s — day %d", ev.title, m.exp.day),
		body:  ev.body,
		opts:  opts,
		cellX: ev.x, cellY: ev.y,
	}
}

// resolveExpChoice applies the chosen option: turn back, flat delay,
// and the pre-rolled risk.
func (m *model) resolveExpChoice(arg int) {
	e := m.exp
	if e == nil || e.pending == nil || arg < 0 || arg >= len(e.pending.choices) {
		return
	}
	ev := e.pending
	e.pending = nil
	c := ev.choices[arg]
	if c.turnBack {
		m.exp = nil
		m.expGen++
		m.status = "the expedition turns back — the road was not worth it"
		return
	}
	e.delay += c.delay
	m.status = c.safeText
	if c.riskOdds > 0 && ev.roll < c.riskOdds {
		e.delay += c.riskDelay
		m.status = c.riskText
	}
	// Keep the outcome on the status line through the next stretch of
	// march days — one tick later it would otherwise vanish.
	e.note = m.status
	e.noteUntil = e.day + 10
	// A current ridden can't outrun the caravan's own arithmetic:
	// never let accumulated delay pull the next arrival before today.
	if e.pos < len(e.path)-1 && e.arrive[e.pos+1]+e.delay < e.day {
		e.delay = e.day - e.arrive[e.pos+1]
	}
}
