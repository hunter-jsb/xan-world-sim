package world

import "fmt"

// Fate — the sealed record of one simulated millennium, and the
// bridge between the engine's two motions. A slice at kya K covers
// exactly one deep-time step (sliceYears = 1000 = 1 ka); its fate is
// what the next step of deep time remembers: the halls still
// standing, the tells of the halls that fell (with their stories),
// and the era's annals. Because the sim is a pure function of
// (seed, kya) and the player only ever observes it, the fate of a
// watched slice IS the canonical fate — sealing what you watched and
// computing it headlessly produce the same record, so deep time
// simulated from within a simulation stays consistent with deep time
// scrubbed from outside.
//
// What survives a millennium is deliberately lossy: places, names,
// and stories persist; stances, grievances, and wars wash out — every
// age swears its oaths anew, and the polity re-forms around the
// rivers, the old halls, and the tells.

// FateSeat is a hall alive when the age was sealed.
type FateSeat struct {
	X     int64  `json:"x"`
	Y     int64  `json:"y"`
	Tier  int64  `json:"tier"`
	Name  string `json:"name"`
	House string `json:"house"`
}

// FateRuin is a hall lost during the age — a tell in the making.
// Story is the chronicle line that recorded the loss.
type FateRuin struct {
	X     int64  `json:"x"`
	Y     int64  `json:"y"`
	Name  string `json:"name"`
	House string `json:"house"`
	Year  int    `json:"year"`
	Story string `json:"story"`
}

// FateEvent is one line of the age's annals (the chronicle's majors).
type FateEvent struct {
	Year int    `json:"year"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Fate is the distilled terminal record of the canonical slice at
// Kya. It covers the millennium (Kya, Kya−1] and is folded into
// every world generated at kya < Kya.
type Fate struct {
	Seed   int64       `json:"seed"`
	Kya    int         `json:"kya"`
	Seats  []FateSeat  `json:"seats"`
	Ruins  []FateRuin  `json:"ruins"`
	Annals []FateEvent `json:"annals"`
}

// TellInfo names an ancient ruin on a generated map — a fate ruin
// that survived reconciliation against the new era's geography.
type TellInfo struct {
	ID     int64
	Name   string
	X, Y   int64
	Story  string // how it fell, from the old age's chronicle
	EraKya int    // the kya of the age that recorded it
}

// EpochReached reports whether the slice has run its full millennium
// — the moment its fate can become the next step of deep time.
func (s *Sim) EpochReached() bool { return s.Months >= sliceYears*monthsPerYear }

// DistillFate reduces a running slice to what the next age will
// remember. Nominally called at or after the epoch mark (year 1000);
// the record is whatever stands and whatever fell by that moment.
func DistillFate(s *Sim) Fate {
	f := Fate{Seed: s.W.Seed, Kya: s.W.Kya}
	for i := range s.W.Seats {
		st := s.W.Seats[i]
		f.Seats = append(f.Seats, FateSeat{X: st.X, Y: st.Y, Tier: st.Tier, Name: st.Name, House: s.house[i]})
	}
	f.Ruins = append(f.Ruins, s.fallen...)
	for _, e := range s.Log {
		if e.Major {
			f.Annals = append(f.Annals, FateEvent{Year: e.Year, Kind: e.Kind, Text: e.Text})
		}
	}
	return f
}

// CanonicalFate computes the fate of the unwatched slice at kya on
// the world the chain implies — the same record a player would seal
// after watching that millennium run.
func CanonicalFate(seed int64, kya int, chain []Fate) Fate {
	s := NewSimWithFates(seed, kya, chain)
	s.StepMonths(sliceYears * monthsPerYear)
	return DistillFate(s)
}

// fateSeatViable reports whether the ground under an old hall still
// holds it in this era — land a seat can stand on, per the same
// terrain judgment travel uses.
func fateSeatViable(kind int64) bool {
	switch kind {
	case RegionCradle, RegionForest, RegionTundra, RegionFoothill,
		RegionAgraria, RegionAgrariaUpland, RegionDoab, RegionMarsh:
		return true
	}
	return false
}

// foldFates lays the remembered ages onto the equilibrium world.
// Seats come from the most recent applicable fate only — the chain
// is cumulative by construction (each sealed slice ran on a world
// already carrying the ages before it), so the latest record lists
// every survivor. Tells accumulate across all applicable fates (a
// fate records only its own age's losses). Reconciliation drops what
// the new era's geography has taken: drowned, frozen, buried ground
// keeps its dead in the annals but not on the map.
func (w *World) foldFates(fates []Fate) {
	var latest *Fate
	for i := range fates {
		f := &fates[i]
		if f.Kya <= w.Kya {
			continue // that age is this moment's future
		}
		if latest == nil || f.Kya < latest.Kya {
			latest = f
		}
	}
	if latest == nil {
		return
	}

	g := gridOf(w.Regions)
	seatAt := make(map[[2]int64]bool, len(w.Seats))
	for _, s := range w.Seats {
		seatAt[[2]int64{s.X, s.Y}] = true
	}
	nearSeat := func(x, y int64) bool {
		for dy := int64(-1); dy <= 1; dy++ {
			for dx := int64(-1); dx <= 1; dx++ {
				if seatAt[[2]int64{x + dx, y + dy}] {
					return true
				}
			}
		}
		return false
	}

	// The old halls that outlasted their age: folded wherever the new
	// equilibrium didn't already raise a hall on or beside the spot,
	// and the ground still holds. They join the world before roads
	// and the polity, so the new age builds around them.
	var folded []FateSeat
	for _, fs := range latest.Seats {
		if nearSeat(fs.X, fs.Y) {
			continue // the new age already holds this ground
		}
		if !fateSeatViable(g.regionAt([2]int{int(fs.X), int(fs.Y)})) {
			continue // the sea, the ice, or the stone took it
		}
		folded = append(folded, fs)
		seatAt[[2]int64{fs.X, fs.Y}] = true
	}
	for _, fs := range folded {
		tier := fs.Tier
		if tier == RegionCapital {
			// The crown is sworn anew each age: the old capital folds
			// in as a great Tributary — and chooseCapital, which runs
			// after the fold, may well crown it again on merit.
			tier = RegionSeat
		}
		w.Regions = setRegionAt(w.Regions, fs.X, fs.Y, tier)
		w.Seats = append(w.Seats, NamedSeat{X: fs.X, Y: fs.Y, Tier: tier, Name: fs.Name})
	}
	// A river may now run through an old hall's cell — the seat
	// stands, the channel yields the glyph (placeSeats' own rule).
	if len(folded) > 0 {
		filtered := w.Rivers[:0]
		for _, r := range w.Rivers {
			if seatAt[[2]int64{r.X, r.Y}] {
				continue
			}
			filtered = append(filtered, r)
		}
		w.Rivers = filtered
	}

	// The tells: every recorded loss whose ground survives, oldest
	// age first; living halls stand on their own bones unmarked.
	for fi := len(fates) - 1; fi >= 0; fi-- {
		f := &fates[fi]
		if f.Kya <= w.Kya {
			continue
		}
		for _, fr := range f.Ruins {
			p := [2]int64{fr.X, fr.Y}
			if seatAt[p] {
				continue // built upon its bones
			}
			if !fateSeatViable(g.regionAt([2]int{int(fr.X), int(fr.Y)})) {
				continue
			}
			seatAt[p] = true // one tell per cell
			w.Regions = setRegionAt(w.Regions, fr.X, fr.Y, RegionRuin)
			w.Tells = append(w.Tells, TellInfo{
				ID:     int64(len(w.Tells) + 1),
				Name:   fr.Name,
				X:      fr.X,
				Y:      fr.Y,
				Story:  fmt.Sprintf("an age gone: %s", fr.Story),
				EraKya: f.Kya,
			})
		}
	}
}

// setRegionAt flips the region of one cell in a regions slice and
// returns it (generation-time counterpart of the sim's setRegion).
func setRegionAt(regions []RegionCell, x, y, regionID int64) []RegionCell {
	for i := range regions {
		if regions[i].X == x && regions[i].Y == y {
			regions[i].RegionID = regionID
			break
		}
	}
	return regions
}

// NewSimWithFates is NewSim on the world the chain implies. The
// latest fate also seeds continuity the map can't carry: a hall that
// stood at the seal keeps its house, and the chronicle opens with
// the dawn of the new age.
func NewSimWithFates(seed int64, kya int, chain []Fate) *Sim {
	s := newSimOn(GenerateWithFates(seed, kya, chain), seed, kya)
	var latest *Fate
	for i := range chain {
		f := &chain[i]
		if f.Kya <= kya {
			continue
		}
		if latest == nil || f.Kya < latest.Kya {
			latest = f
		}
	}
	if latest == nil {
		return s
	}
	houseOf := make(map[[2]int64]string, len(latest.Seats))
	for _, fs := range latest.Seats {
		houseOf[[2]int64{fs.X, fs.Y}] = fs.House
	}
	carried := 0
	for i := range s.W.Seats {
		if h, ok := houseOf[[2]int64{s.W.Seats[i].X, s.W.Seats[i].Y}]; ok && h != "" {
			s.house[i] = h
			carried++
		}
	}
	var x, y int64
	if s.capitalIdx >= 0 {
		x, y = s.W.Seats[s.capitalIdx].X, s.W.Seats[s.capitalIdx].Y
	}
	s.Log = append(s.Log, SimEvent{
		Kind: "epoch", Major: true, X: x, Y: y, Cause: -1,
		Text: fmt.Sprintf("a new age dawns — %d houses carry their names across the millennium, and %d tells mark the old one",
			carried, len(s.W.Tells)),
	})
	return s
}
