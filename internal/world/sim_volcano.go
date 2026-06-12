package world

import (
	"fmt"
	"sort"
)

// The slice's share of the seed's volcanic timeline. The schedule is
// drawn once in geology.go and consumed twice: deep time integrates
// every eruption into the bedrock, and a slice replays the ones whose
// sub-ka moment falls inside its own millennium — same vent, same
// moment, same size, so what a slice witnesses is exactly what deep
// time records. Between the scheduled fires, restless vents grumble
// (tremors and ash — flavor with no permanent mark, safely rng-driven
// because nothing persistent depends on it).

const (
	// ashAllegianceHit is the loyalty cost of an eruption's ash years
	// for every hall in the vent's shadow — failed harvests are the
	// crown's fault, as far as a hungry hall is concerned.
	ashAllegianceHit = 0.04

	// eruptionUnrestYears is how long an eruption owns nearby unrest
	// in the chronicle's causal web (mirrors the succession-crisis
	// window).
	eruptionUnrestYears = 25

	// Restless vents (heat ≥ grumbleHeatMin) grumble at
	// volcanoGrumbleChance per year — tremors, ash plumes, night
	// glow. Pure chronicle flavor.
	grumbleHeatMin       = 0.5
	volcanoGrumbleChance = 0.015

	// fertStrengthK scales how much a hall's granary feeds its
	// realm's war strength: a hall on the best ground counts half
	// again as much as one scratching at bare rock.
	fertStrengthK = 0.5
)

// simEruption is one scheduled eruption landing inside the slice.
type simEruption struct {
	month int // s.Months at which it fires
	vi    int // index into s.volcanoes
	size  int // 1..3 — cone growth and flow length, as in deep time
}

// initVolcanoes wires the slice's volcanic state from the world's
// timeline: every site (born or not) as a pressure projector, and
// the eruption queue for this millennium.
func (s *Sim) initVolcanoes(seed int64, kya int) {
	w := s.W
	n := len(w.volcanoSites)
	s.volcanoes = make([]lairSite, n)
	s.volcanoHeat = make([]float64, n)
	s.volcanoName = make([]string, n)
	s.volcanoBorn = make([]bool, n)
	s.volcanoEventIdx = make([]int, n)
	bornAt := make(map[[2]int64]VolcanoInfo, len(w.Volcanoes))
	for _, v := range w.Volcanoes {
		bornAt[[2]int64{v.X, v.Y}] = v
	}
	for i, site := range w.volcanoSites {
		x, y := int64(site.x), int64(site.y)
		s.volcanoes[i] = volcanoPressureSite(x, y)
		s.volcanoName[i] = generateName(nameSeedForCell(seed, x, y) + volcanoNameSalt)
		s.volcanoEventIdx[i] = -1
		if v, ok := bornAt[[2]int64{x, y}]; ok {
			s.volcanoBorn[i] = true
			s.volcanoHeat[i] = volcanoHeatAt(v.LastAgo)
		}
	}
	// Eruptions whose true moment falls inside this slice: schedule
	// entries at kya−1 land at year (1 − frac) × 1000 of the slice.
	for _, e := range w.volcanoSched {
		if e.kya != kya-1 {
			continue
		}
		site := w.volcanoSites[e.v]
		year := int(float64(sliceYears) * (1 - eruptionFrac(seed, site, e.kya)))
		if year < 1 {
			year = 1
		}
		if year > sliceYears {
			year = sliceYears
		}
		month := (year-1)*monthsPerYear + 1 +
			int(geoHash01(seed, site.x, site.y, 2000000+e.kya)*float64(monthsPerYear))
		s.eruptions = append(s.eruptions, simEruption{month: month, vi: e.v, size: e.size})
	}
	sort.Slice(s.eruptions, func(i, j int) bool {
		if s.eruptions[i].month != s.eruptions[j].month {
			return s.eruptions[i].month < s.eruptions[j].month
		}
		return s.eruptions[i].vi < s.eruptions[j].vi
	})
}

// VolcanoHeat reports the live heat of the vent at (x, y), 0 if none
// — the danger map's volcano analog of LairActivity.
func (s *Sim) VolcanoHeat(x, y int64) float64 {
	for i, v := range s.volcanoes {
		if v.X == x && v.Y == y {
			return s.volcanoHeat[i]
		}
	}
	return 0
}

// stepVolcanoes fires any eruption whose month has come and lets
// restless vents grumble. Returns true when the map changed (lava,
// buried halls) and borders must re-settle.
func (s *Sim) stepVolcanoes(emit emitFn) bool {
	changed := false
	for len(s.eruptions) > 0 && s.eruptions[0].month <= s.Months {
		e := s.eruptions[0]
		s.eruptions = s.eruptions[1:]
		s.fireEruption(e, emit)
		changed = true
	}
	for vi := range s.volcanoes {
		if s.volcanoHeat[vi] < grumbleHeatMin {
			continue
		}
		if s.rng.Float64() < volcanoGrumbleChance/monthsPerYear {
			v := s.volcanoes[vi]
			texts := [3]string{
				"the earth trembles beneath %s",
				"ash veils the sky over %s",
				"fire glows in the night above %s",
			}
			emit(s.volcanoEventIdx[vi], SimEvent{Kind: "tremor", X: v.X, Y: v.Y,
				Text: fmt.Sprintf(texts[s.rng.Intn(3)], s.volcanoName[vi])})
		}
	}
	return changed
}

// fireEruption is one scheduled eruption landing on the slice: the
// summit becomes (or remains) a volcano, a lava flow walks steepest
// descent over the slice's terrain burying halls and lairs in its
// path, ash thins allegiance across the vent's shadow, and the vent
// burns at full heat in the shared pressure field. Deep time owns
// the authoritative bedrock version of the same flow.
func (s *Sim) fireEruption(e simEruption, emit emitFn) {
	v := s.volcanoes[e.vi]
	name := s.volcanoName[e.vi]
	txt := fmt.Sprintf("%s wakes — fire crowns the mountain", name)
	if !s.volcanoBorn[e.vi] {
		txt = fmt.Sprintf("the peak splits open — %s is born in fire", name)
	}
	flowLen := lavaFlowBase + lavaFlowPerSize*e.size
	idx := emit(-1, SimEvent{Kind: "eruption", Major: true, X: v.X, Y: v.Y, Text: txt,
		Detail: fmt.Sprintf("ash darkens the harvests for years; the flow runs %d cells", flowLen)})
	s.volcanoBorn[e.vi] = true
	s.volcanoHeat[e.vi] = 1
	s.volcanoEventIdx[e.vi] = idx
	s.setRegion(v.X, v.Y, RegionVolcano)
	s.fertAt[[2]int64{v.X, v.Y}] = 0

	g := gridOf(s.W.Regions)
	x, y := v.X, v.Y
	for i := 0; i < flowLen; i++ {
		bx, by := int64(-1), int64(-1)
		best := g.elevAt([2]int{int(x), int(y)})
		for _, d := range dirs8 {
			nx, ny := x+int64(d[0]), y+int64(d[1])
			if !inBounds(int(nx), int(ny)) {
				continue
			}
			if e := g.elevAt([2]int{int(nx), int(ny)}); e < best {
				best, bx, by = e, nx, ny
			}
		}
		if bx < 0 {
			break // ponded in its own hollow
		}
		x, y = bx, by
		switch RegionKind(g.regionAt([2]int{int(x), int(y)})) {
		case "sea_brine", "sea_eastern", "lake", "drowned":
			i = flowLen // quenched at the water
			continue
		}
		if s.buryHallAt(x, y, name, idx, emit) {
			break // the flow dies at the capital's walls — the crown holds
		}
		s.buryLairAt(x, y, name, idx, emit)
		s.setRegion(x, y, RegionLava)
		s.fertAt[[2]int64{x, y}] = 0
	}

	// Ash falls on every hall in the vent's shadow.
	for i := range s.W.Seats {
		if i == s.capitalIdx {
			continue // the capital's stores are deep; its allegiance is itself
		}
		st := &s.W.Seats[i]
		if lairPressureAt(v, st.X, st.Y, 1) > 0 {
			st.Allegiance -= ashAllegianceHit
			if st.Allegiance < 0 {
				st.Allegiance = 0
			}
		}
	}
	s.recomputeLairNoted()
}

// buryHallAt entombs any hall on the flow cell. No RuinSite is left —
// there is nothing under the stone to resettle. Returns true only for
// the capital, which is never removed: the flow stops at its walls.
func (s *Sim) buryHallAt(x, y int64, volcano string, cause int, emit emitFn) bool {
	for i := range s.W.Seats {
		st := s.W.Seats[i]
		if st.X != x || st.Y != y {
			continue
		}
		if i == s.capitalIdx {
			return true
		}
		detail := ""
		realmID := st.RealmID
		if realmID != 0 {
			detail = fmt.Sprintf("%s is left with %d halls",
				s.realmTitle(realmID), s.realmHallCount(realmID)-1)
		}
		idx := emit(cause, SimEvent{Kind: "ruin", Major: true, X: x, Y: y,
			Text: fmt.Sprintf("the flows of %s entomb %s — the hall of House %s is gone beneath the stone",
				volcano, st.Name, s.house[i]),
			Detail: detail})
		s.removeSeat(i)
		if realmID != 0 {
			s.maybeDissolve(realmID, emit, idx)
		}
		return false
	}
	return false
}

// buryLairAt removes any lair the flow crosses — even a dragon yields
// to the mountain itself.
func (s *Sim) buryLairAt(x, y int64, volcano string, cause int, emit emitFn) {
	for i := range s.lairs {
		l := s.lairs[i]
		if l.X != x || l.Y != y {
			continue
		}
		if s.lairNoted[i] {
			emit(cause, SimEvent{Kind: "lair", X: x, Y: y,
				Text: fmt.Sprintf("the flows of %s swallow %s — the %s will not return", volcano, l.Name, l.Kind)})
		}
		s.deadLairs = append(s.deadLairs, [2]int64{x, y})
		s.lairs = append(s.lairs[:i], s.lairs[i+1:]...)
		s.activity = append(s.activity[:i], s.activity[i+1:]...)
		s.lairState = append(s.lairState[:i], s.lairState[i+1:]...)
		s.lairNoted = append(s.lairNoted[:i], s.lairNoted[i+1:]...)
		s.lairEventIdx = append(s.lairEventIdx[:i], s.lairEventIdx[i+1:]...)
		return
	}
}
