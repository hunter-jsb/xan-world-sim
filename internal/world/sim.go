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
	// activityWalkSigma drives each den's raid-activity random walk
	// (reflected into [0, activityMax], starting at 1 = the static
	// generation-time level). A walk step of 0.10/yr crosses the 0.5
	// gap from normal to rampant in ~25 years on average — dragon
	// tempers turn on generational timescales.
	activityWalkSigma = 0.10
	activityMax       = 2.0

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
// ones (stances, dragon tempers) just stream past.
type SimEvent struct {
	Year  int
	Kind  string // "stance", "secede", "swear", "dissolve", "dragon", "epoch"
	Text  string
	X, Y  int64
	Major bool
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

	// Per-den state, indexed like W.Dens.
	activity []float64 // raid-activity multiplier in [0, activityMax]
	denState []int     // 0 normal, 1 rampant, -1 dormant

	nextRealmID int64
}

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
	if s.capitalIdx >= 0 {
		capSeat := w.Seats[s.capitalIdx]
		dist := w.logisticCostFrom([][2]int{{int(capSeat.X), int(capSeat.Y)}})
		for i := range w.Seats {
			st := w.Seats[i]
			if L := dist[st.Y][st.X]; L >= 0 {
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

	s.activity = make([]float64, len(w.Dens))
	s.denState = make([]int, len(w.Dens))
	for i := range s.activity {
		s.activity[i] = 1
	}
	return s
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

	s.stepDragons(emit)
	s.stepAllegiance(emit)
	changed := s.stepMembership(emit)
	if changed {
		s.W.Territory = s.W.Territory[:0]
		s.W.claimTerritory()
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

// stepDragons walks each den's raid activity and recomputes every
// seat's pressure as the strongest activity-weighted raid falloff. At
// activity 1 this reproduces applyDragonPressure exactly.
func (s *Sim) stepDragons(emit func(SimEvent)) {
	for i := range s.activity {
		a := s.activity[i] + s.rng.NormFloat64()*activityWalkSigma
		if a < 0 {
			a = -a // reflect at the floor: a sleeping dragon still dreams
		}
		if a > activityMax {
			a = 2*activityMax - a
		}
		s.activity[i] = a

		d := s.W.Dens[i]
		switch {
		case s.denState[i] != 1 && a >= rampantEnter:
			s.denState[i] = 1
			emit(SimEvent{Kind: "dragon", X: d.X, Y: d.Y,
				Text: fmt.Sprintf("the dragon of %s stirs — raid-fires on the horizon", d.Name)})
		case s.denState[i] != -1 && a <= dormantEnter:
			s.denState[i] = -1
			emit(SimEvent{Kind: "dragon", X: d.X, Y: d.Y,
				Text: fmt.Sprintf("the dragon of %s falls dormant — the skies clear", d.Name)})
		case s.denState[i] == 1 && a < rampantExit:
			s.denState[i] = 0
			emit(SimEvent{Kind: "dragon", X: d.X, Y: d.Y,
				Text: fmt.Sprintf("the raids out of %s subside", d.Name)})
		case s.denState[i] == -1 && a > dormantExit:
			s.denState[i] = 0
			emit(SimEvent{Kind: "dragon", X: d.X, Y: d.Y,
				Text: fmt.Sprintf("wings over %s again", d.Name)})
		}
	}

	for i := range s.W.Seats {
		st := &s.W.Seats[i]
		var p float64
		for j, d := range s.W.Dens {
			dx := int(st.X - d.X)
			if dx < 0 {
				dx = -dx
			}
			dy := int(st.Y - d.Y)
			if dy < 0 {
				dy = -dy
			}
			if cheb := max(dx, dy); cheb < dragonRaidRadius {
				if c := float64(dragonRaidRadius-cheb) * s.activity[j]; c > p {
					p = c
				}
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
		var name string
		for _, r := range s.W.Realms {
			if r.ID == bestRealm {
				name = r.Name
			}
		}
		emit(SimEvent{Kind: "secede", Major: true, X: st.X, Y: st.Y,
			Text: fmt.Sprintf("%s renounces the crown and joins the league of %s", st.Name, name)})
		return
	}

	st.RealmID = s.nextRealmID
	s.W.Realms = append(s.W.Realms, Realm{
		ID:    s.nextRealmID,
		Name:  st.Name,
		SeatX: st.X,
		SeatY: st.Y,
	})
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

	var crownName string
	for _, r := range s.W.Realms {
		if r.ID == s.crownID {
			crownName = r.Name
		}
	}
	emit(SimEvent{Kind: "swear", Major: true, X: st.X, Y: st.Y,
		Text: fmt.Sprintf("%s swears to the crown of %s", st.Name, crownName)})

	if oldRealm == 0 {
		return
	}
	for j := range s.W.Seats {
		if s.W.Seats[j].RealmID == oldRealm {
			return // the league lives on
		}
	}
	for j, r := range s.W.Realms {
		if r.ID == oldRealm {
			s.W.Realms = append(s.W.Realms[:j], s.W.Realms[j+1:]...)
			emit(SimEvent{Kind: "dissolve", Major: true, X: r.SeatX, Y: r.SeatY,
				Text: fmt.Sprintf("the league of %s dissolves", r.Name)})
			return
		}
	}
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
