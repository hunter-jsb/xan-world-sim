package world

import (
	"fmt"
	"math/rand"
)

// The simulation layer animates one slice of deep time. Deep time
// scrubs *between* worlds: each kya is an independent equilibrium
// snapshot, politics as the geography would settle it. A Sim pins one
// (seed, kya) slice and runs years *inside* it — geography and climate
// hold still (a millennium is below geological resolution), while the
// political layer comes alive: dragons stir and quiet, allegiance
// drifts around its geographic equilibrium, seats change stance,
// defect from or swear to the crown, leagues form and dissolve, and
// territory is re-claimed after every shift.
//
// Determinism contract: NewSim(seed, kya) stepped N years always
// produces the same world state and the same event log. The step
// function draws from one seeded RNG in a fixed order (dens, then
// seats) and processes membership changes in seat order, so replaying
// a slice replays its history exactly.

// Simulation pacing constants. Calibrated by probe (see sim_test.go
// lineage): targets are a stance shift somewhere on the map every few
// years, a defection or swearing once or twice a century, and each
// dragon changing temper a few times per millennium.
const (
	// Each lair's raid activity does a random walk (reflected into
	// [0, activityMax], starting at 1 = the static generation-time
	// level). A dragon's walk step of 0.10/yr crosses the 0.5 gap from
	// normal to rampant in ~25 years on average — tempers turn on
	// generational timescales. Lesser tiers are steadier: a drake
	// swarm waxes with broods, and a colonial rookery averages out its
	// individuals' moods.
	dragonWalkSigma = 0.10
	drakeWalkSigma  = 0.08
	wyvernWalkSigma = 0.06
	activityMax     = 2.0

	// Rampant/dormant bands carry hysteresis so a den hovering at the
	// boundary doesn't flap between tempers every other year.
	rampantEnter = 1.5
	rampantExit  = 1.2
	dormantEnter = 0.3
	dormantExit  = 0.6

	// allegianceDrift is the fraction of the gap to equilibrium closed
	// per year — a court answers changed circumstances within a decade
	// or two, not instantly. allegianceNoise is yearly court politics
	// too small to name.
	allegianceDrift = 0.08
	allegianceNoise = 0.006

	// temperament is a per-seat slow random walk (clamped to
	// ±temperamentMax) standing in for what geography can't see:
	// heirs, feuds, faith. It guarantees no two centuries play alike
	// even under a quiet sky.
	temperamentWalkSigma = 0.01
	temperamentMax       = 0.08

	// stanceHysteresis: a stance boundary must be cleared by this
	// margin before the seat's reputation actually changes.
	stanceHysteresis = 0.015

	// Reigns: each hall's lord rules 8–39 years; successions re-roll
	// the seat's temperament (a new lord is a new disposition) and
	// reset its oath streaks (loyalty is sworn to a person). Smooth
	// successions pass unchronicled; a failed line (crisis) shakes the
	// hall's allegiance, and a contested *crown* succession ripples
	// doubt through every sworn hall.
	reignMinYears          = 8
	reignSpanYears         = 32
	successionCrisisChance = 0.12
	successionCrisisDoubt  = 0.08
	crownCrisisDoubt       = 0.04

	// houseSeedSalt offsets a seat's house-name stream from its own
	// name stream (both derive from the same cell coordinates).
	houseSeedSalt = 7331

	// Ruin: only dragonfire razes halls — pressure 15 exceeds what any
	// drake swarm or rookery can project (drake max ≈ 9.3 at rampant)
	// and needs a genuinely rampant dragon (activity ≥ 1.5) within ~2
	// cells, sustained ruinYears straight. Marches and Headwaters are
	// built for exactly this life — "battle-hardened," the wall — and
	// withstand marchHardening× more before they break; without it,
	// every near-mountain hall burns in the first decades of every
	// slice. The crown itself never falls (the capital sits in the
	// calm heartland by construction).
	ruinPressure   = 15.0
	ruinYears      = 12
	marchHardening = 1.3

	// Founding: each realm has a small yearly chance to raise a new
	// hall, given a calm site (local pressure ≤ foundCalmPressure)
	// inside its own territory, clear of existing halls and ruins by
	// seatMinSep. River-adjacent sites found Tributaries; the rest
	// found Outholds. Ruins inside the territory are resettled first —
	// the old name returns, under a new house.
	foundChance       = 0.0005
	foundCalmPressure = 2.0
	seatMinSep        = 4

	// Wars: grievance is heat between two realms — secessions and
	// captures pour it in, shared borders add slow friction, time
	// bleeds it off (≈2%/yr). Each bordered pair risks war in
	// proportion to its grievance; the stronger side declares (wars
	// start when the strong think they'll win). The score drifts with
	// relative strength plus fortune; at ±captureScore a border hall
	// changes hands by force. Wars end by exhaustion (maxWarYears) or,
	// often, after a capture. Only crowned ages fight wars — ice-age
	// clan raids are below this layer's resolution.
	grievanceSecede    = 0.5
	grievanceCapture   = 0.4
	grievanceDecay     = 0.98
	borderFriction     = 0.0008
	warChance          = 0.01 // × grievance, per candidate pair per year
	warMarchGrievance  = 0.15 // grievance that carries armies across the wilds
	warDriftK          = 0.06
	warNoise           = 0.08
	captureScore       = 1.0
	maxWarYears        = 30
	warEndAfterCapture = 0.6
	raidPeriod         = 4
	raidDoubt          = 0.02

	// Membership changes demand sustained conviction, not a bad year.
	// Both thresholds mirror crownThreshold — the same boundary
	// formRealms partitions by at generation — offset by a hysteresis
	// gap so borderline seats don't flap. Defection is faster than
	// swearing: rebellion is quick, trust is slow ("geography itself
	// makes them... easier to rebel").
	defectThreshold = crownThreshold - 0.05
	defectYears     = 15
	swearThreshold  = crownThreshold + 0.05
	swearYears      = 25
)

// sliceYears is the nominal span of one deep-time slice — kya
// resolution is 1000 years, so at year 1000 the sim marks the epoch.
const sliceYears = 1000

// SimEvent is one entry in a slice's chronicle. Major events are the
// ones the TUI interrupts for (membership and epoch changes); minor
// ones (stances, lair tempers) just stream past.
type SimEvent struct {
	Year  int
	Kind  string // "stance", "secede", "swear", "dissolve", "lair", "epoch"
	Text  string
	X, Y  int64
	Major bool
}

// lairTemperText holds each lair kind's four temper-transition lines:
// rampant-enter, dormant-enter, rampant-exit, dormant-exit.
var lairTemperText = map[string][4]string{
	"dragon": {
		"the dragon of %s stirs — raid-fires on the horizon",
		"the dragon of %s falls dormant — the skies clear",
		"the raids out of %s subside",
		"wings over %s again",
	},
	"drakes": {
		"the drakes of %s swarm the lowlands",
		"the drakes of %s go to ground",
		"the drake-swarms around %s thin",
		"drakes prowl from %s again",
	},
	"wyverns": {
		"the wyverns of %s wheel in war-flocks",
		"the rookery at %s falls silent",
		"the war-flocks over %s scatter",
		"wyverns ride the winds from %s again",
	},
}

// Sim is a running simulation over one frozen slice of deep time.
type Sim struct {
	W    *World
	Year int
	Log  []SimEvent

	rng        *rand.Rand
	capitalIdx int   // index into W.Seats; -1 = no crown this age
	crownID    int64 // realm ID of the crown; 0 = none

	// Per-seat state, indexed like W.Seats.
	base        []float64 // allegiance before pressure; -1 = unreachable
	temperament []float64
	stance      []string
	lowStreak   []int // consecutive years in the autonomous band (crown seats)
	highStreak  []int // consecutive years above swearThreshold (independents)

	// Heritage lines, indexed like W.Seats: the ruling house of each
	// hall, the year it took the hall, and the year the current lord's
	// reign ends.
	house      []string
	houseSince []int
	reignEnd   []int

	// lineage records the origin of realms founded during the slice
	// (realm ID → chronicle note); realms that predate it have none.
	lineage map[int64]string

	// Rise and fall: ruinStreak counts consecutive years under
	// ruinPressure (indexed like W.Seats); ruins are the halls lost
	// this slice; patches are map-cell kind changes for the TUI to
	// splice into its render data; capDist is the capital's logistic
	// field, kept so founded seats can score their allegiance base;
	// riverAt marks river cells for founding-site tier choice.
	ruinStreak []int
	ruins      []RuinSite
	patches    []CellPatch
	capDist    [][]int
	riverAt    map[[2]int64]bool

	// Per-lair state, indexed like lairs (all three tiers flattened).
	lairs     []lairSite
	activity  []float64 // raid-activity multiplier in [0, activityMax]
	lairState []int     // 0 normal, 1 rampant, -1 dormant
	lairNoted []bool    // some seat lies in raid range — tempers make the chronicle

	// Wars: active conflicts, the standing grievances between realm
	// pairs (keyed by sorted ID pair), and the current border-contact
	// counts derived from territory.
	wars      []war
	grievance map[[2]int64]float64
	borders   map[[2]int64]int

	// Dynamic borders (sim_borders.go): the static logistic cost of
	// every cell, the contested marchland set, a version stamp the
	// TUI uses to notice border refreshes, and the reusable Dijkstra
	// buffer (one refresh runs one search per hall).
	costGrid    [][]int
	contested   map[[2]int64]bool
	terrVersion int
	fieldBuf    [][]int

	nextRealmID int64
}

// war is one running conflict; Score > 0 favors the declarer A.
type war struct {
	A, B  int64 // realm IDs; A declared
	Start int
	Score float64
}

// Wars exposes the active conflicts (for the TUI's realm dossiers).
func (s *Sim) Wars() []war { return s.wars }

// RealmDisplayName names a realm for the TUI ("" if it fell).
func (s *Sim) RealmDisplayName(id int64) string { return s.realmName(id) }

// AtWar reports whether the two realms are currently fighting.
func (s *Sim) AtWar(a, b int64) bool {
	for _, w := range s.wars {
		if (w.A == a && w.B == b) || (w.A == b && w.B == a) {
			return true
		}
	}
	return false
}

// pairKey orders two realm IDs into a canonical map key.
func pairKey(a, b int64) [2]int64 {
	if a < b {
		return [2]int64{a, b}
	}
	return [2]int64{b, a}
}

// realmName returns the name of the realm with the given ID.
func (s *Sim) realmName(id int64) string {
	for _, r := range s.W.Realms {
		if r.ID == id {
			return r.Name
		}
	}
	return ""
}

// realmTitle is the realm's styled name for event text: the crown is
// "the crown of X", everyone else "the league of X".
func (s *Sim) realmTitle(id int64) string {
	if id == s.crownID {
		return "the crown of " + s.realmName(id)
	}
	return "the league of " + s.realmName(id)
}

// LairActivity reports the current activity multiplier of the lair at
// (x, y), or 1 if no lair is there — the TUI scales the expedition
// danger map by this, so route costs shift with the times.
func (s *Sim) LairActivity(x, y int64) float64 {
	for i, l := range s.lairs {
		if l.X == x && l.Y == y {
			return s.activity[i]
		}
	}
	return 1
}

// HouseAt returns the ruling house of the seat at (x, y) and the year
// it took the hall ("" if no seat is there).
func (s *Sim) HouseAt(x, y int64) (string, int) {
	for i := range s.W.Seats {
		if s.W.Seats[i].X == x && s.W.Seats[i].Y == y {
			return s.house[i], s.houseSince[i]
		}
	}
	return "", 0
}

// RealmLineage returns the chronicle's origin note for a realm founded
// during this slice ("" for realms that predate it).
func (s *Sim) RealmLineage(realmID int64) string {
	return s.lineage[realmID]
}

// RuinSite is a hall lost during this slice — it stays on the map as
// a ruin until (unless) someone raises it again.
type RuinSite struct {
	X, Y int64
	Name string
	Year int
}

// CellPatch is one map-cell kind change the sim has made (a hall
// razed or raised); the TUI splices these into its render data.
type CellPatch struct {
	X, Y int64
	Kind string
}

// Ruins lists the slice's fallen halls (resettled ones drop off).
func (s *Sim) Ruins() []RuinSite { return s.ruins }

// CellPatches returns every map-cell change so far, in order. The TUI
// tracks how many it has applied; the list only grows.
func (s *Sim) CellPatches() []CellPatch { return s.patches }

// NewSim generates the slice's world and prepares it for stepping.
// The world is generated fresh (never loaded) so the sim's geography
// is exactly the deterministic snapshot for (seed, kya).
func NewSim(seed int64, kya int) *Sim {
	w := Generate(seed, kya)
	s := &Sim{
		W: &w,
		// The sim's own RNG is independent of worldgen's: mixing kya in
		// keeps two slices of the same seed from replaying each other's
		// court politics.
		rng:        rand.New(rand.NewSource(seed*1000003 + int64(kya)*7919 + 1)),
		capitalIdx: -1,
	}
	for i := range w.Seats {
		if w.Seats[i].Tier == RegionCapital {
			s.capitalIdx = i
			break
		}
	}
	for _, r := range w.Realms {
		if r.IsCrown {
			s.crownID = r.ID
		}
		if r.ID >= s.nextRealmID {
			s.nextRealmID = r.ID + 1
		}
	}
	if s.nextRealmID == 0 {
		s.nextRealmID = 1
	}

	s.base = make([]float64, len(w.Seats))
	s.temperament = make([]float64, len(w.Seats))
	s.stance = make([]string, len(w.Seats))
	s.lowStreak = make([]int, len(w.Seats))
	s.highStreak = make([]int, len(w.Seats))
	s.ruinStreak = make([]int, len(w.Seats))
	if s.capitalIdx >= 0 {
		capSeat := w.Seats[s.capitalIdx]
		s.capDist = w.logisticCostFrom([][2]int{{int(capSeat.X), int(capSeat.Y)}})
		for i := range w.Seats {
			st := w.Seats[i]
			if L := s.capDist[st.Y][st.X]; L >= 0 {
				s.base[i] = allegianceBase(L, st.Tier)
			} else {
				s.base[i] = -1 // beyond the crown's world
			}
			s.stance[i] = AllegianceStance(st.Allegiance)
		}
	} else {
		for i := range s.base {
			s.base[i] = -1
			s.stance[i] = AllegianceStance(0)
		}
	}
	s.riverAt = make(map[[2]int64]bool, len(w.Rivers))
	for _, r := range w.Rivers {
		s.riverAt[[2]int64{r.X, r.Y}] = true
	}
	s.grievance = make(map[[2]int64]float64)
	s.contested = make(map[[2]int64]bool)
	s.buildCostGrid()
	s.recomputeBorders()

	// Heritage lines: each hall's founding house is as deterministic as
	// its name (same cell coordinates, salted stream); the sitting
	// lords are mid-reign when the slice opens.
	s.house = make([]string, len(w.Seats))
	s.houseSince = make([]int, len(w.Seats))
	s.reignEnd = make([]int, len(w.Seats))
	s.lineage = make(map[int64]string)
	for i := range w.Seats {
		st := w.Seats[i]
		s.house[i] = generateName(nameSeedForCell(seed, st.X, st.Y) + houseSeedSalt)
		s.reignEnd[i] = 1 + s.rng.Intn(reignMinYears+reignSpanYears)
	}

	s.lairs = w.lairSites()
	s.activity = make([]float64, len(s.lairs))
	s.lairState = make([]int, len(s.lairs))
	s.lairNoted = make([]bool, len(s.lairs))
	for i := range s.activity {
		s.activity[i] = 1
	}
	s.recomputeLairNoted()
	return s
}

// recomputeLairNoted refreshes which lairs have a seat in raid range —
// the set shifts when halls fall or rise.
func (s *Sim) recomputeLairNoted() {
	for i, l := range s.lairs {
		s.lairNoted[i] = false
		for _, st := range s.W.Seats {
			if lairPressureAt(l, st.X, st.Y, 1) > 0 {
				s.lairNoted[i] = true
				break
			}
		}
	}
}

// StepYear advances the slice one year and returns the year's events
// (also appended to the chronicle). Order is fixed for determinism:
// dragons stir, pressure lands, courts drift, stances settle,
// membership breaks last.
func (s *Sim) StepYear() []SimEvent {
	s.Year++
	var events []SimEvent
	emit := func(e SimEvent) {
		e.Year = s.Year
		events = append(events, e)
	}

	s.stepLairs(emit)
	s.stepAllegiance(emit)
	s.stepSuccessions(emit)
	changed := s.stepMembership(emit)
	if s.stepRuins(emit) {
		changed = true
	}
	if s.stepFoundings(emit) {
		changed = true
	}
	if s.stepWars(emit) {
		changed = true
	}
	// Borders re-settle after any structural change, and on a steady
	// cadence regardless — conviction and war fortune move them even
	// when no hall changed hands.
	if changed || s.Year%borderRefreshYears == 0 {
		s.reclaimTerritory()
		s.recomputeBorders()
	}

	if s.Year == sliceYears {
		var x, y int64
		if s.capitalIdx >= 0 {
			x, y = s.W.Seats[s.capitalIdx].X, s.W.Seats[s.capitalIdx].Y
		}
		emit(SimEvent{
			Kind: "epoch", Major: true, X: x, Y: y,
			Text: "a thousand years have passed — the slice has run its course; deeper change belongs to deep time",
		})
	}

	s.Log = append(s.Log, events...)
	return events
}

// lairWalkSigma is the per-tier volatility of a lair's activity walk.
func lairWalkSigma(kind string) float64 {
	switch kind {
	case "dragon":
		return dragonWalkSigma
	case "drakes":
		return drakeWalkSigma
	default: // wyverns
		return wyvernWalkSigma
	}
}

// stepLairs walks every lair's raid activity and recomputes every
// seat's pressure as the strongest activity-weighted raid falloff
// (lairPressureAt — at activity 1 this reproduces applyDragonPressure
// exactly). Temper changes enter the chronicle only for lairs with a
// seat in raid range: the courts record what reaches their walls, the
// rest is weather.
func (s *Sim) stepLairs(emit func(SimEvent)) {
	for i, l := range s.lairs {
		a := s.activity[i] + s.rng.NormFloat64()*lairWalkSigma(l.Kind)
		if a < 0 {
			a = -a // reflect at the floor: a sleeping dragon still dreams
		}
		if a > activityMax {
			a = 2*activityMax - a
		}
		s.activity[i] = a

		temper := -1
		switch {
		case s.lairState[i] != 1 && a >= rampantEnter:
			s.lairState[i] = 1
			temper = 0
		case s.lairState[i] != -1 && a <= dormantEnter:
			s.lairState[i] = -1
			temper = 1
		case s.lairState[i] == 1 && a < rampantExit:
			s.lairState[i] = 0
			temper = 2
		case s.lairState[i] == -1 && a > dormantExit:
			s.lairState[i] = 0
			temper = 3
		}
		// Courts chronicle every dragon mood, but for lesser lairs only
		// the threat's arrival (rampant/dormant onset) is news — its
		// passing is taken for granted.
		if temper >= 0 && s.lairNoted[i] && (l.Kind == "dragon" || temper < 2) {
			emit(SimEvent{Kind: "lair", X: l.X, Y: l.Y,
				Text: fmt.Sprintf(lairTemperText[l.Kind][temper], l.Name)})
		}
	}

	s.recomputePressure()
}

// recomputePressure rescores every seat against the current lair
// activities. With all activities at 1 this reproduces the
// generator's applyDragonPressure exactly (pinned by test).
func (s *Sim) recomputePressure() {
	for i := range s.W.Seats {
		st := &s.W.Seats[i]
		var p float64
		for j, l := range s.lairs {
			if c := lairPressureAt(l, st.X, st.Y, s.activity[j]); c > p {
				p = c
			}
		}
		st.Pressure = p
	}
}

// stepAllegiance drifts every reachable seat toward its current
// equilibrium — the geographic base, colored by temperament, taxed by
// this year's dragon pressure — and reports stance changes.
func (s *Sim) stepAllegiance(emit func(SimEvent)) {
	if s.capitalIdx < 0 {
		return // no crown to be loyal to; the courts sleep
	}
	for i := range s.W.Seats {
		st := &s.W.Seats[i]
		if i == s.capitalIdx {
			st.Allegiance = 1 // the crown is loyal to itself
			continue
		}
		if s.base[i] < 0 {
			continue // unreachable: beyond the crown's world
		}
		t := s.temperament[i] + s.rng.NormFloat64()*temperamentWalkSigma
		s.temperament[i] = min(max(t, -temperamentMax), temperamentMax)

		e := s.base[i] + s.temperament[i] - pressureAllegiancePenalty*st.Pressure
		a := st.Allegiance + allegianceDrift*(e-st.Allegiance) + s.rng.NormFloat64()*allegianceNoise
		st.Allegiance = min(max(a, 0), 1)

		if next := stickyStance(st.Allegiance, s.stance[i]); next != s.stance[i] {
			verb := "rises to"
			if stanceRank(next) < stanceRank(s.stance[i]) {
				verb = "slips to"
			}
			s.stance[i] = next
			emit(SimEvent{Kind: "stance", X: st.X, Y: st.Y,
				Text: fmt.Sprintf("%s %s %s allegiance", st.Name, verb, next)})
		}
	}
}

// stepSuccessions turns over the generations. When a reign ends the
// heir takes the hall: temperament re-rolls (a new lord is a new
// disposition) and oath streaks reset (loyalty is sworn to a person,
// not a map). Smooth successions pass unchronicled — the record keeps
// ruptures: a failed line shakes its hall's allegiance, and a failed
// line *on the throne* ripples doubt through every sworn hall.
func (s *Sim) stepSuccessions(emit func(SimEvent)) {
	for i := range s.W.Seats {
		if s.Year < s.reignEnd[i] {
			continue
		}
		st := &s.W.Seats[i]
		s.reignEnd[i] = s.Year + reignMinYears + s.rng.Intn(reignSpanYears)
		s.temperament[i] = (s.rng.Float64()*2 - 1) * temperamentMax
		s.lowStreak[i], s.highStreak[i] = 0, 0
		if s.rng.Float64() >= successionCrisisChance {
			continue // the heir is sound; the hall barely notices
		}
		old := s.house[i]
		s.house[i] = generateName(s.rng.Int63())
		s.houseSince[i] = s.Year
		if i == s.capitalIdx {
			for j := range s.W.Seats {
				if j != s.capitalIdx && s.W.Seats[j].RealmID == s.crownID && s.base[j] >= 0 {
					s.W.Seats[j].Allegiance = max(s.W.Seats[j].Allegiance-crownCrisisDoubt, 0)
				}
			}
			emit(SimEvent{Kind: "succession", Major: true, X: st.X, Y: st.Y,
				Text: fmt.Sprintf("the line of %s fails on the throne of %s — House %s takes the crown, and doubt ripples outward",
					old, st.Name, s.house[i])})
			continue
		}
		if s.base[i] >= 0 {
			st.Allegiance = max(st.Allegiance-successionCrisisDoubt, 0)
		}
		emit(SimEvent{Kind: "succession", X: st.X, Y: st.Y,
			Text: fmt.Sprintf("the line of %s fails in %s — House %s takes the hall", old, st.Name, s.house[i])})
	}
}

// stepMembership breaks and forges realm bonds: crown seats that have
// sat below defectThreshold for defectYears renounce; independents
// that have held above swearThreshold for swearYears bend the knee.
// Returns whether any membership changed (territory must then be
// re-claimed).
func (s *Sim) stepMembership(emit func(SimEvent)) bool {
	if s.crownID == 0 {
		return false
	}
	changed := false
	for i := range s.W.Seats {
		st := &s.W.Seats[i]
		if i == s.capitalIdx || s.base[i] < 0 {
			continue
		}
		if st.RealmID == s.crownID {
			s.highStreak[i] = 0
			if st.Allegiance < defectThreshold {
				s.lowStreak[i]++
			} else {
				s.lowStreak[i] = 0
			}
			if s.lowStreak[i] >= defectYears {
				s.lowStreak[i] = 0
				s.secede(i, emit)
				changed = true
			}
		} else {
			s.lowStreak[i] = 0
			if st.Allegiance >= swearThreshold {
				s.highStreak[i]++
			} else {
				s.highStreak[i] = 0
			}
			if s.highStreak[i] >= swearYears {
				s.highStreak[i] = 0
				s.swear(i, emit)
				changed = true
			}
		}
	}
	return changed
}

// secede pulls seat i out of the crown realm: it joins the nearest
// independent league within enclaveRadius (any member hall counts as a
// door), or stands alone as a new league bearing its own name.
func (s *Sim) secede(i int, emit func(SimEvent)) {
	st := &s.W.Seats[i]
	dist := s.W.logisticCostFrom([][2]int{{int(st.X), int(st.Y)}})

	bestRealm := int64(0)
	bestD := enclaveRadius + 1
	for _, r := range s.W.Realms {
		if r.IsCrown {
			continue
		}
		for j := range s.W.Seats {
			o := s.W.Seats[j]
			if o.RealmID != r.ID {
				continue
			}
			if d := dist[o.Y][o.X]; d >= 0 && d < bestD {
				bestD = d
				bestRealm = r.ID
			}
		}
	}

	if bestRealm != 0 {
		st.RealmID = bestRealm
		s.grievance[pairKey(bestRealm, s.crownID)] += grievanceSecede
		emit(SimEvent{Kind: "secede", Major: true, X: st.X, Y: st.Y,
			Text: fmt.Sprintf("%s renounces the crown and joins the league of %s", st.Name, s.realmName(bestRealm))})
		return
	}

	st.RealmID = s.nextRealmID
	s.W.Realms = append(s.W.Realms, Realm{
		ID:    s.nextRealmID,
		Name:  st.Name,
		SeatX: st.X,
		SeatY: st.Y,
	})
	s.lineage[s.nextRealmID] = fmt.Sprintf("sundered from the crown of %s in year %d, under House %s",
		s.realmName(s.crownID), s.Year, s.house[i])
	s.grievance[pairKey(s.nextRealmID, s.crownID)] += grievanceSecede
	s.nextRealmID++
	emit(SimEvent{Kind: "secede", Major: true, X: st.X, Y: st.Y,
		Text: fmt.Sprintf("%s renounces the crown and stands alone — the league of %s", st.Name, st.Name)})
}

// swear moves seat i into the crown realm. If its old league is left
// without a single hall, the league dissolves.
func (s *Sim) swear(i int, emit func(SimEvent)) {
	st := &s.W.Seats[i]
	oldRealm := st.RealmID
	st.RealmID = s.crownID
	emit(SimEvent{Kind: "swear", Major: true, X: st.X, Y: st.Y,
		Text: fmt.Sprintf("%s swears to the crown of %s", st.Name, s.realmName(s.crownID))})

	s.maybeDissolve(oldRealm, emit)
}

// maybeDissolve removes a realm that no longer holds a single hall.
func (s *Sim) maybeDissolve(realmID int64, emit func(SimEvent)) {
	if realmID == 0 {
		return
	}
	for j := range s.W.Seats {
		if s.W.Seats[j].RealmID == realmID {
			return // the league lives on
		}
	}
	for j, r := range s.W.Realms {
		if r.ID == realmID {
			s.W.Realms = append(s.W.Realms[:j], s.W.Realms[j+1:]...)
			emit(SimEvent{Kind: "dissolve", Major: true, X: r.SeatX, Y: r.SeatY,
				Text: fmt.Sprintf("the league of %s dissolves", r.Name)})
			return
		}
	}
}

// setRegion flips one map cell's region, keeps the cost grid honest,
// and records the patch for the TUI's render data.
func (s *Sim) setRegion(x, y, regionID int64) {
	for i := range s.W.Regions {
		rc := &s.W.Regions[i]
		if rc.X == x && rc.Y == y {
			rc.RegionID = regionID
			break
		}
	}
	s.patchCostGrid(x, y, regionID)
	s.patches = append(s.patches, CellPatch{X: x, Y: y, Kind: RegionKind(regionID)})
}

// removeSeat splices seat i out of the world and every parallel
// array. The capital is never removed, so capitalIdx only shifts.
func (s *Sim) removeSeat(i int) {
	s.W.Seats = append(s.W.Seats[:i], s.W.Seats[i+1:]...)
	s.base = append(s.base[:i], s.base[i+1:]...)
	s.temperament = append(s.temperament[:i], s.temperament[i+1:]...)
	s.stance = append(s.stance[:i], s.stance[i+1:]...)
	s.lowStreak = append(s.lowStreak[:i], s.lowStreak[i+1:]...)
	s.highStreak = append(s.highStreak[:i], s.highStreak[i+1:]...)
	s.ruinStreak = append(s.ruinStreak[:i], s.ruinStreak[i+1:]...)
	s.house = append(s.house[:i], s.house[i+1:]...)
	s.houseSince = append(s.houseSince[:i], s.houseSince[i+1:]...)
	s.reignEnd = append(s.reignEnd[:i], s.reignEnd[i+1:]...)
	if i < s.capitalIdx {
		s.capitalIdx--
	}
}

// stepRuins breaks halls that have burned too long: a seat held at or
// above ruinPressure for ruinYears is sacked — struck from the living
// world, its cell scarred to RegionRuin, its realm dissolved if it was
// the last hall. Only dragonfire reaches the threshold.
func (s *Sim) stepRuins(emit func(SimEvent)) bool {
	var doomed []int
	for i := range s.W.Seats {
		if i == s.capitalIdx {
			s.ruinStreak[i] = 0 // the crown holds
			continue
		}
		threshold := ruinPressure
		if tier := s.W.Seats[i].Tier; tier == RegionMarch || tier == RegionHeadwater {
			threshold *= marchHardening // the wall is built for this
		}
		if s.W.Seats[i].Pressure >= threshold {
			s.ruinStreak[i]++
		} else {
			s.ruinStreak[i] = 0
		}
		if s.ruinStreak[i] >= ruinYears {
			doomed = append(doomed, i)
		}
	}
	if len(doomed) == 0 {
		return false
	}
	var realmsTouched []int64
	for _, i := range doomed {
		st := s.W.Seats[i]
		s.ruins = append(s.ruins, RuinSite{X: st.X, Y: st.Y, Name: st.Name, Year: s.Year})
		s.setRegion(st.X, st.Y, RegionRuin)
		if st.RealmID != 0 {
			realmsTouched = append(realmsTouched, st.RealmID)
		}
		emit(SimEvent{Kind: "ruin", Major: true, X: st.X, Y: st.Y,
			Text: fmt.Sprintf("dragonfire takes %s — the hall of House %s lies in ruins", st.Name, s.house[i])})
	}
	for j := len(doomed) - 1; j >= 0; j-- {
		s.removeSeat(doomed[j])
	}
	for _, id := range realmsTouched {
		s.maybeDissolve(id, emit)
	}
	s.recomputeLairNoted()
	return true
}

// stepFoundings lets a lucky realm raise a new hall on calm ground
// inside its own territory — a ruin of the slice is resettled first
// (the old name returns under a new house), otherwise the best fresh
// site: river-adjacent founds a Tributary, open land an Outhold.
func (s *Sim) stepFoundings(emit func(SimEvent)) bool {
	if len(s.W.Realms) == 0 {
		return false
	}
	var winners []int64
	for _, r := range s.W.Realms {
		if s.rng.Float64() < foundChance {
			winners = append(winners, r.ID)
		}
	}
	if len(winners) == 0 {
		return false
	}
	owner := make(map[[2]int64]int64, len(s.W.Territory))
	for _, tc := range s.W.Territory {
		owner[[2]int64{tc.X, tc.Y}] = tc.RealmID
	}
	founded := false
	for _, realmID := range winners {
		if s.foundSeat(realmID, owner, emit) {
			founded = true
		}
	}
	return founded
}

// sitePressure is the static raid exposure of a cell (activity-scaled).
func (s *Sim) sitePressure(x, y int64) float64 {
	var p float64
	for j, l := range s.lairs {
		if c := lairPressureAt(l, x, y, s.activity[j]); c > p {
			p = c
		}
	}
	return p
}

// clearOfSeats reports whether (x, y) keeps seatMinSep Chebyshev
// distance from every living hall and every ruin.
func (s *Sim) clearOfSeats(x, y int64) bool {
	cheb := func(ax, ay int64) int64 {
		dx, dy := x-ax, y-ay
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		return max(dx, dy)
	}
	for _, st := range s.W.Seats {
		if cheb(st.X, st.Y) < seatMinSep {
			return false
		}
	}
	for _, r := range s.ruins {
		if cheb(r.X, r.Y) < seatMinSep {
			return false
		}
	}
	return true
}

// foundSeat raises one hall for the realm, preferring its oldest
// unsettled ruin, then the best fresh site in its territory (scored:
// river-adjacent +2, cradle/agraria +1; ties to scan order).
func (s *Sim) foundSeat(realmID int64, owner map[[2]int64]int64, emit func(SimEvent)) bool {
	nearRiver := func(x, y int64) bool {
		if s.riverAt[[2]int64{x, y}] {
			return true
		}
		for _, d := range dirs8 {
			if s.riverAt[[2]int64{x + int64(d[0]), y + int64(d[1])}] {
				return true
			}
		}
		return false
	}

	// A ruin of the slice, inside the realm's territory, calm enough.
	for ri, ruin := range s.ruins {
		if owner[[2]int64{ruin.X, ruin.Y}] != realmID || s.sitePressure(ruin.X, ruin.Y) > foundCalmPressure {
			continue
		}
		s.ruins = append(s.ruins[:ri], s.ruins[ri+1:]...)
		s.raiseHall(realmID, ruin.X, ruin.Y, ruin.Name, nearRiver(ruin.X, ruin.Y), true, emit)
		return true
	}

	bestScore := -1
	var bx, by int64
	bestRiver := false
	for _, rc := range s.W.Regions {
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionAgraria, RegionDoab:
		default:
			continue
		}
		p := [2]int64{rc.X, rc.Y}
		if owner[p] != realmID || s.riverAt[p] || !s.clearOfSeats(rc.X, rc.Y) {
			continue
		}
		if s.sitePressure(rc.X, rc.Y) > foundCalmPressure {
			continue
		}
		score := 0
		river := nearRiver(rc.X, rc.Y)
		if river {
			score += 2
		}
		if rc.RegionID == RegionCradle || rc.RegionID == RegionAgraria {
			score++
		}
		if score > bestScore {
			bestScore, bx, by, bestRiver = score, rc.X, rc.Y, river
		}
	}
	if bestScore < 0 {
		return false
	}
	s.raiseHall(realmID, bx, by, generateName(nameSeedForCell(s.W.Seed, bx, by)), bestRiver, false, emit)
	return true
}

// raiseHall appends the new seat with all its parallel state and
// flips its map cell to the seat tier.
func (s *Sim) raiseHall(realmID int64, x, y int64, name string, onRiver, resettled bool, emit func(SimEvent)) {
	tier := RegionOuthold
	if onRiver {
		tier = RegionSeat
	}
	s.setRegion(x, y, tier)

	pressure := s.sitePressure(x, y)
	base := -1.0
	allegiance := 0.0
	if s.capitalIdx >= 0 {
		if L := s.capDist[y][x]; L >= 0 {
			base = allegianceBase(L, tier)
			allegiance = min(max(base-pressureAllegiancePenalty*pressure, 0), 1)
		}
	}
	s.W.Seats = append(s.W.Seats, NamedSeat{
		X: x, Y: y, Tier: tier, Name: name,
		Pressure: pressure, RealmID: realmID, Allegiance: allegiance,
	})
	s.base = append(s.base, base)
	s.temperament = append(s.temperament, 0)
	s.stance = append(s.stance, AllegianceStance(allegiance))
	s.lowStreak = append(s.lowStreak, 0)
	s.highStreak = append(s.highStreak, 0)
	s.ruinStreak = append(s.ruinStreak, 0)
	s.house = append(s.house, generateName(s.rng.Int63()))
	s.houseSince = append(s.houseSince, s.Year)
	s.reignEnd = append(s.reignEnd, s.Year+reignMinYears+s.rng.Intn(reignSpanYears))
	s.recomputeLairNoted()

	if resettled {
		emit(SimEvent{Kind: "founding", Major: true, X: x, Y: y,
			Text: fmt.Sprintf("%s is raised again from its ruins by the realm of %s, under House %s",
				name, s.realmName(realmID), s.house[len(s.house)-1])})
		return
	}
	emit(SimEvent{Kind: "founding", Major: true, X: x, Y: y,
		Text: fmt.Sprintf("a new hall rises — %s, of the realm of %s", name, s.realmName(realmID))})
}

// recomputeBorders rebuilds the realm-pair contact counts from the
// territory grid (E and S neighbors only, so each touching pair of
// cells counts once).
func (s *Sim) recomputeBorders() {
	s.borders = make(map[[2]int64]int)
	owner := make(map[[2]int64]int64, len(s.W.Territory))
	for _, tc := range s.W.Territory {
		owner[[2]int64{tc.X, tc.Y}] = tc.RealmID
	}
	for _, tc := range s.W.Territory {
		for _, d := range [2][2]int64{{1, 0}, {0, 1}} {
			if o := owner[[2]int64{tc.X + d[0], tc.Y + d[1]}]; o != 0 && o != tc.RealmID {
				s.borders[pairKey(tc.RealmID, o)]++
			}
		}
	}
}

// realmStrength weighs a realm's halls for war: every hall counts,
// Marches count half again ("battle-hardened"), the capital double.
func (s *Sim) realmStrength(id int64) float64 {
	str := 0.0
	for i := range s.W.Seats {
		if s.W.Seats[i].RealmID != id {
			continue
		}
		str++
		if s.W.Seats[i].Tier == RegionMarch {
			str += 0.5
		}
		if i == s.capitalIdx {
			str++
		}
	}
	return str
}

// frontSeat picks the defender's hall on the war's front — the one
// nearest (Chebyshev) to any of the attacker's halls. skipCapital
// protects the crown's seat from capture (raiders may still reach its
// fields). Returns -1 if either side has no halls.
func (s *Sim) frontSeat(defender, attacker int64, skipCapital bool) int {
	best, bestD := -1, 1<<30
	for i := range s.W.Seats {
		if s.W.Seats[i].RealmID != defender || (skipCapital && i == s.capitalIdx) {
			continue
		}
		for j := range s.W.Seats {
			if s.W.Seats[j].RealmID != attacker {
				continue
			}
			dx := int(s.W.Seats[i].X - s.W.Seats[j].X)
			if dx < 0 {
				dx = -dx
			}
			dy := int(s.W.Seats[i].Y - s.W.Seats[j].Y)
			if dy < 0 {
				dy = -dy
			}
			if d := max(dx, dy); d < bestD {
				best, bestD = i, d
			}
		}
	}
	return best
}

// sortedPairs returns map keys in canonical (a, b) order — map
// iteration is randomized and the war rolls must not be.
func sortedPairs[V any](m map[[2]int64]V) [][2]int64 {
	keys := make([][2]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && (keys[j][0] < keys[j-1][0] ||
			(keys[j][0] == keys[j-1][0] && keys[j][1] < keys[j-1][1])); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// stepWars runs the year's grievances, declarations, campaigns,
// captures, and peaces. Returns whether any seat changed hands.
func (s *Sim) stepWars(emit func(SimEvent)) bool {
	if s.crownID == 0 {
		return false // no crowned order to fight over
	}

	// Grievance bookkeeping: decay what stands, add border friction.
	heat := make(map[[2]int64]bool, len(s.grievance)+len(s.borders))
	for k := range s.grievance {
		heat[k] = true
	}
	for k := range s.borders {
		heat[k] = true
	}
	for _, k := range sortedPairs(heat) {
		g := s.grievance[k] * grievanceDecay
		if n := s.borders[k]; n > 0 {
			g += borderFriction * min(float64(n)/10, 1)
		}
		if g < 0.01 {
			delete(s.grievance, k)
		} else {
			s.grievance[k] = g
		}
	}

	// Declarations: bordered pairs risk war in proportion to their
	// grievance, and a strong grievance (≥ warMarchGrievance) carries
	// armies across the wilds even without a shared border — the
	// crown reconquers its lost halls wherever they stand. The
	// stronger side declares.
	candidates := make(map[[2]int64]bool, len(s.borders)+len(s.grievance))
	for k := range s.borders {
		candidates[k] = true
	}
	for k, g := range s.grievance {
		if g >= warMarchGrievance {
			candidates[k] = true
		}
	}
	for _, k := range sortedPairs(candidates) {
		if s.AtWar(k[0], k[1]) || s.realmName(k[0]) == "" || s.realmName(k[1]) == "" {
			continue
		}
		g := s.grievance[k]
		if g <= 0 || s.rng.Float64() >= warChance*g {
			continue
		}
		a, b := k[0], k[1]
		if s.realmStrength(b) > s.realmStrength(a) {
			a, b = b, a
		}
		s.wars = append(s.wars, war{A: a, B: b, Start: s.Year})
		ra, _ := s.realmSeatXY(a)
		emit(SimEvent{Kind: "war", Major: true, X: ra[0], Y: ra[1],
			Text: fmt.Sprintf("war — %s marches on %s", s.realmTitle(a), s.realmTitle(b))})
	}

	// Campaigns.
	changed := false
	kept := s.wars[:0]
	for _, w := range s.wars {
		nameA, nameB := s.realmName(w.A), s.realmName(w.B)
		if nameA == "" || nameB == "" {
			// One banner dissolved mid-war (sworn away, burned out, or
			// captured whole) — the war ends with it.
			emit(SimEvent{Kind: "peace", Major: true,
				Text: "the war ends — one of its banners is no more"})
			continue
		}
		strA, strB := s.realmStrength(w.A), s.realmStrength(w.B)
		w.Score += warDriftK*(strA-strB)/(strA+strB) + s.rng.NormFloat64()*warNoise

		if age := s.Year - w.Start; age > 0 && age%raidPeriod == 0 {
			defender, attacker := w.B, w.A
			if w.Score < 0 {
				defender, attacker = w.A, w.B
			}
			if ti := s.frontSeat(defender, attacker, false); ti >= 0 {
				st := &s.W.Seats[ti]
				if st.RealmID == s.crownID && ti != s.capitalIdx && s.base[ti] >= 0 {
					// A crown that cannot protect its halls loses them.
					st.Allegiance = max(st.Allegiance-raidDoubt, 0)
				}
				emit(SimEvent{Kind: "raid", X: st.X, Y: st.Y,
					Text: fmt.Sprintf("the war reaches %s — fields burn outside its walls", st.Name)})
			}
		}

		if w.Score >= captureScore || w.Score <= -captureScore {
			winner, loser := w.A, w.B
			if w.Score < 0 {
				winner, loser = w.B, w.A
			}
			if ti := s.frontSeat(loser, winner, true); ti >= 0 {
				st := &s.W.Seats[ti]
				old := st.RealmID
				st.RealmID = winner
				s.lowStreak[ti], s.highStreak[ti] = 0, 0
				s.grievance[pairKey(loser, winner)] += grievanceCapture
				emit(SimEvent{Kind: "capture", Major: true, X: st.X, Y: st.Y,
					Text: fmt.Sprintf("%s falls to %s — House %s bends the knee",
						st.Name, s.realmTitle(winner), s.house[ti])})
				changed = true
				s.maybeDissolve(old, emit)
			}
			w.Score = 0
			if s.rng.Float64() < warEndAfterCapture || s.realmName(loser) == "" {
				s.grievance[pairKey(w.A, w.B)] *= 0.5
				emit(SimEvent{Kind: "peace", Major: true,
					Text: fmt.Sprintf("peace between %s and %s — the borders rest", nameA, nameB)})
				continue
			}
		}

		if s.Year-w.Start >= maxWarYears {
			s.grievance[pairKey(w.A, w.B)] *= 0.5
			emit(SimEvent{Kind: "peace", Major: true,
				Text: fmt.Sprintf("peace between %s and %s — both sides are spent", nameA, nameB)})
			continue
		}
		kept = append(kept, w)
	}
	s.wars = kept
	return changed
}

// realmSeatXY locates a realm's leading hall for event coordinates.
func (s *Sim) realmSeatXY(id int64) ([2]int64, bool) {
	for _, r := range s.W.Realms {
		if r.ID == id {
			return [2]int64{r.SeatX, r.SeatY}, true
		}
	}
	return [2]int64{}, false
}

// stanceRank orders the stance vocabulary from least to most loyal.
func stanceRank(stance string) int {
	switch stance {
	case "autonomous":
		return 0
	case "nominal":
		return 1
	case "tributary":
		return 2
	default: // sworn
		return 3
	}
}

// stickyStance applies stanceHysteresis: the new stance must clear its
// boundary by the margin, or the seat keeps its current reputation —
// reputations change slower than moods.
func stickyStance(a float64, cur string) string {
	raw := AllegianceStance(a)
	if raw == cur {
		return cur
	}
	if stanceRank(raw) > stanceRank(cur) {
		if AllegianceStance(a-stanceHysteresis) == cur {
			return cur
		}
	} else {
		if AllegianceStance(a+stanceHysteresis) == cur {
			return cur
		}
	}
	return raw
}
