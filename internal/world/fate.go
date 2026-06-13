package world

import (
	"fmt"
	"sort"
)

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

// FateSeat is a hall alive when the age was sealed. HouseAges counts
// the consecutive ages its house has held it, the sealing age
// included — deep roots resist succession crises in the ages after.
type FateSeat struct {
	X         int64  `json:"x"`
	Y         int64  `json:"y"`
	Tier      int64  `json:"tier"`
	Name      string `json:"name"`
	House     string `json:"house"`
	HouseAges int    `json:"house_ages"`
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

// FateRealm is a polity standing at the seal. Realms are keyed by
// name across ages (a realm's name is its leading hall's, and hall
// names are cell-keyed) — a realm whose name re-forms in the next
// age continues the lineage and counts another age.
type FateRealm struct {
	Name    string `json:"name"`
	IsCrown bool   `json:"is_crown"`
	Age     int    `json:"age"` // ages this realm has stood, this one included
}

// FateLair is the state of a lair at the seal: its raid activity
// carries across the dawn (a rampant dragon does not calm for a
// calendar), and a buried lair stays buried — the mountain keeps
// what it takes.
type FateLair struct {
	X        int64   `json:"x"`
	Y        int64   `json:"y"`
	Kind     string  `json:"kind"`
	Activity float64 `json:"activity"`
	Buried   bool    `json:"buried"`
}

// FateGrudge is a standing grievance between realms at the seal,
// keyed by realm names. Oaths wash out between ages; blood feuds
// smolder on as embers.
type FateGrudge struct {
	A    string  `json:"a"`
	B    string  `json:"b"`
	Heat float64 `json:"heat"`
}

// Fate is the distilled terminal record of the canonical slice at
// Kya. It covers the millennium (Kya, Kya−1] and is folded into
// every world generated at kya < Kya. Age is the record's ordinal —
// the first sealed age of a seed is 1.
type Fate struct {
	Seed    int64        `json:"seed"`
	Kya     int          `json:"kya"`
	Age     int          `json:"age"`
	Seats   []FateSeat   `json:"seats"`
	Ruins   []FateRuin   `json:"ruins"`
	Annals  []FateEvent  `json:"annals"`
	Realms  []FateRealm  `json:"realms"`
	Lairs   []FateLair   `json:"lairs"`
	Grudges []FateGrudge `json:"grudges"`
}

// grudgeMinHeat is the grievance a feud needs at the seal to be
// remembered; grudgeEmberK is how much of it survives the dawn.
const (
	grudgeMinHeat = 0.3
	grudgeEmberK  = 0.5
)

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
	f := Fate{Seed: s.W.Seed, Kya: s.W.Kya, Age: s.ageNumber}
	for i := range s.W.Seats {
		st := s.W.Seats[i]
		f.Seats = append(f.Seats, FateSeat{X: st.X, Y: st.Y, Tier: st.Tier, Name: st.Name,
			House: s.house[i], HouseAges: s.houseAges[i] + 1})
	}
	f.Ruins = append(f.Ruins, s.fallen...)
	for _, e := range s.Log {
		if e.Major {
			f.Annals = append(f.Annals, FateEvent{Year: e.Year, Kind: e.Kind, Text: e.Text})
		}
	}
	for _, r := range s.W.Realms {
		age := r.Age
		if age < 1 {
			age = 1 // realms born mid-slice are in their first age
		}
		f.Realms = append(f.Realms, FateRealm{Name: r.Name, IsCrown: r.IsCrown, Age: age})
	}
	for i, l := range s.lairs {
		f.Lairs = append(f.Lairs, FateLair{X: l.X, Y: l.Y, Kind: l.Kind, Activity: s.activity[i]})
	}
	for _, d := range s.deadLairs {
		f.Lairs = append(f.Lairs, FateLair{X: d[0], Y: d[1], Buried: true})
	}
	for pair, heat := range s.grievance {
		if heat < grudgeMinHeat {
			continue
		}
		a, b := s.realmName(pair[0]), s.realmName(pair[1])
		if a == "" || b == "" {
			continue // a feud dies with its realm
		}
		f.Grudges = append(f.Grudges, FateGrudge{A: a, B: b, Heat: heat})
	}
	sortGrudges(f.Grudges)
	return f
}

// sortGrudges orders grudges deterministically (map iteration above
// is not) — by names, then heat.
func sortGrudges(gs []FateGrudge) {
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].A != gs[j].A {
			return gs[i].A < gs[j].A
		}
		if gs[i].B != gs[j].B {
			return gs[i].B < gs[j].B
		}
		return gs[i].Heat > gs[j].Heat
	})
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

	// The mountain keeps what it takes: lairs the last age saw buried
	// don't re-form at the same peaks. Runs against the freshly placed
	// lair lists, so feature counts and region cells stay paired.
	for _, fl := range latest.Lairs {
		if !fl.Buried {
			continue
		}
		for i, d := range w.Dens {
			if d.X == fl.X && d.Y == fl.Y {
				w.Dens = append(w.Dens[:i], w.Dens[i+1:]...)
				w.Regions = setRegionAt(w.Regions, fl.X, fl.Y, RegionMountain)
				break
			}
		}
		for i, n := range w.Nests {
			if n.X == fl.X && n.Y == fl.Y {
				w.Nests = append(w.Nests[:i], w.Nests[i+1:]...)
				w.Regions = setRegionAt(w.Regions, fl.X, fl.Y, RegionFoothill)
				break
			}
		}
		for i, r := range w.Rookeries {
			if r.X == fl.X && r.Y == fl.Y {
				w.Rookeries = append(w.Rookeries[:i], w.Rookeries[i+1:]...)
				w.Regions = setRegionAt(w.Regions, fl.X, fl.Y, RegionCliff)
				break
			}
		}
	}
}

// foldRealmLineage runs after the polity forms: a realm whose name
// re-forms across the dawn (the same leading hall, since names are
// cell-keyed) continues its line and counts another age. Everything
// else about the polity is sworn anew — lineage is the memory,
// allegiance is the present.
func (w *World) foldRealmLineage(fates []Fate) {
	var latest *Fate
	for i := range fates {
		f := &fates[i]
		if f.Kya <= w.Kya {
			continue
		}
		if latest == nil || f.Kya < latest.Kya {
			latest = f
		}
	}
	if latest == nil {
		return
	}
	ageOf := make(map[string]int, len(latest.Realms))
	for _, fr := range latest.Realms {
		ageOf[fr.Name] = fr.Age
	}
	for i := range w.Realms {
		if age, ok := ageOf[w.Realms[i].Name]; ok {
			w.Realms[i].Age = age + 1
		}
	}
}

// ageOrdinal spells an age number for chronicle prose.
func AgeOrdinal(n int) string {
	switch n {
	case 1:
		return "first"
	case 2:
		return "second"
	case 3:
		return "third"
	case 4:
		return "fourth"
	case 5:
		return "fifth"
	case 6:
		return "sixth"
	case 7:
		return "seventh"
	case 8:
		return "eighth"
	case 9:
		return "ninth"
	}
	return fmt.Sprintf("%dth", n)
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
// latest fate also seeds the continuity the map can't carry: houses
// keep their halls (and their roots), dragons keep their tempers,
// old enemies keep their embers, and the chronicle opens with the
// dawn of a numbered age.
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
	s.ageNumber = latest.Age + 1

	houseOf := make(map[[2]int64]FateSeat, len(latest.Seats))
	for _, fs := range latest.Seats {
		houseOf[[2]int64{fs.X, fs.Y}] = fs
	}
	carried := 0
	for i := range s.W.Seats {
		if fs, ok := houseOf[[2]int64{s.W.Seats[i].X, s.W.Seats[i].Y}]; ok && fs.House != "" {
			s.house[i] = fs.House
			s.houseAges[i] = fs.HouseAges
			carried++
		}
	}

	// Dragons don't calm for a calendar: surviving lairs wake with
	// the activity they held at the seal.
	lairAt := make(map[[2]int64]FateLair, len(latest.Lairs))
	for _, fl := range latest.Lairs {
		if !fl.Buried {
			lairAt[[2]int64{fl.X, fl.Y}] = fl
		}
	}
	for i, l := range s.lairs {
		if fl, ok := lairAt[[2]int64{l.X, l.Y}]; ok {
			s.activity[i] = fl.Activity
		}
	}

	// Embers of the old feuds: grievance between realms whose names
	// crossed the dawn opens warm.
	realmByName := make(map[string]int64, len(s.W.Realms))
	for _, r := range s.W.Realms {
		realmByName[r.Name] = r.ID
	}
	for _, gr := range latest.Grudges {
		a, aok := realmByName[gr.A]
		b, bok := realmByName[gr.B]
		if !aok || !bok || a == b {
			continue
		}
		if heat := gr.Heat * grudgeEmberK; heat > s.grievance[pairKey(a, b)] {
			s.grievance[pairKey(a, b)] = heat
		}
	}

	var x, y int64
	if s.capitalIdx >= 0 {
		x, y = s.W.Seats[s.capitalIdx].X, s.W.Seats[s.capitalIdx].Y
	}
	s.Log = append(s.Log, SimEvent{
		Kind: "epoch", Major: true, X: x, Y: y, Cause: -1,
		Text: fmt.Sprintf("the %s age dawns — %d houses carry their names across the millennium, and %d tells mark the old one",
			AgeOrdinal(s.ageNumber), carried, len(s.W.Tells)),
	})
	return s
}
