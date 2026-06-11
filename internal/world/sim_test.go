package world

import (
	"fmt"
	"strings"
	"testing"
)

// simFingerprint captures everything the sim mutates — seat politics,
// realms, territory, and the chronicle — at full float precision.
func simFingerprint(s *Sim) string {
	var b strings.Builder
	fmt.Fprintf(&b, "year=%d\n", s.Year)
	for i, st := range s.W.Seats {
		fmt.Fprintf(&b, "seat %s a=%v p=%v r=%d house=%s since=%d\n",
			st.Name, st.Allegiance, st.Pressure, st.RealmID, s.house[i], s.houseSince[i])
	}
	for _, r := range s.W.Realms {
		fmt.Fprintf(&b, "realm %d %s crown=%v\n", r.ID, r.Name, r.IsCrown)
	}
	fmt.Fprintf(&b, "territory=%d\n", len(s.W.Territory))
	for _, r := range s.ruins {
		fmt.Fprintf(&b, "ruin %s (%d,%d) y%d\n", r.Name, r.X, r.Y, r.Year)
	}
	fmt.Fprintf(&b, "patches=%d\n", len(s.patches))
	for _, e := range s.Log {
		fmt.Fprintf(&b, "y%d [%s] %s @(%d,%d) major=%v\n", e.Year, e.Kind, e.Text, e.X, e.Y, e.Major)
	}
	return b.String()
}

// TestSim_Deterministic pins the simulation's contract: replaying a
// slice replays its history — same state, same chronicle, exactly.
func TestSim_Deterministic(t *testing.T) {
	run := func() string {
		s := NewSim(42, 0)
		for y := 0; y < 600; y++ {
			s.StepYear()
		}
		return simFingerprint(s)
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("two runs of the same slice diverged:\n--- first ---\n%s\n--- second ---\n%s", a, b)
	}
}

// TestSim_Invariants steps slices across seeds and climates and holds
// them to the same polity invariants the generator is held to, at
// every century mark — the sim may move the politics, never break it.
func TestSim_Invariants(t *testing.T) {
	cases := []struct {
		seed int64
		kya  int
	}{
		{1, 0}, {42, 0}, {7, 100}, {1, KyaOldWorld}, {42, KyaOldWorld},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("seed%d_kya%d", c.seed, c.kya), func(t *testing.T) {
			s := NewSim(c.seed, c.kya)
			lastYear := 0
			for y := 1; y <= 1200; y++ {
				for _, e := range s.StepYear() {
					if e.Year < lastYear {
						t.Fatalf("event year %d after year %d — chronicle out of order", e.Year, lastYear)
					}
					lastYear = e.Year
					if e.Text == "" {
						t.Errorf("year %d: empty %s event text", e.Year, e.Kind)
					}
				}
				if y%100 == 0 {
					checkPolity(t, *s.W)
					checkSimArrays(t, s)
					if t.Failed() {
						t.Fatalf("polity invariants broken at year %d", y)
					}
				}
			}
		})
	}
}

// TestSim_EventsFire pins the calibration: over one millennium a
// living world must produce dragon tempers, stance shifts, and
// membership changes in both directions, plus the epoch mark.
func TestSim_EventsFire(t *testing.T) {
	s := NewSim(42, 0)
	counts := map[string]int{}
	var epoch *SimEvent
	for y := 0; y < 1000; y++ {
		for _, e := range s.StepYear() {
			counts[e.Kind]++
			if e.Kind == "epoch" {
				ev := e
				epoch = &ev
			}
			switch e.Kind {
			case "secede", "swear", "dissolve", "epoch", "ruin", "founding", "war", "capture", "peace":
				if !e.Major {
					t.Errorf("year %d: %s event not Major", e.Year, e.Kind)
				}
			case "stance", "lair", "raid":
				if e.Major {
					t.Errorf("year %d: %s event marked Major", e.Year, e.Kind)
				}
			}
		}
	}
	for _, kind := range []string{"lair", "stance", "secede", "swear"} {
		if counts[kind] == 0 {
			t.Errorf("no %q events in a millennium — dynamics too quiet (counts: %v)", kind, counts)
		}
	}
	if epoch == nil {
		t.Error("no epoch event at the millennium mark")
	} else if epoch.Year != sliceYears {
		t.Errorf("epoch event at year %d, want %d", epoch.Year, sliceYears)
	}
}

// TestSim_NoCrownAge: under the ice there is no capital and no crown —
// the sim must run clean with the courts asleep (dragons may still
// stir), and never invent politics the generator didn't.
func TestSim_NoCrownAge(t *testing.T) {
	s := NewSim(42, KyaOldWorld)
	if s.capitalIdx != -1 {
		t.Fatalf("LGM slice has a capital (seat %d) — climate coupling broken", s.capitalIdx)
	}
	for y := 0; y < 300; y++ {
		for _, e := range s.StepYear() {
			// Lairs stir, lords die, and halls may burn or rise under
			// the ice too — but no crown politics: no stances, no
			// oaths, no realms moving by choice.
			switch e.Kind {
			case "lair", "succession", "ruin", "founding", "dissolve":
			default:
				t.Errorf("year %d: %s event in a crownless age: %s", e.Year, e.Kind, e.Text)
			}
		}
	}
	for _, st := range s.W.Seats {
		if st.Allegiance != 0 {
			t.Errorf("seat %q allegiance %g in a crownless age, want 0", st.Name, st.Allegiance)
		}
	}
	checkPolity(t, *s.W)
}

// TestSim_Wars: grievance must become war, war must capture and must
// end. Every war that began has ended (or still stands at the close);
// no standing war references a dead realm.
func TestSim_Wars(t *testing.T) {
	s := NewSim(42, 0)
	counts := map[string]int{}
	for y := 0; y < 3000; y++ {
		for _, e := range s.StepYear() {
			counts[e.Kind]++
		}
	}
	for _, kind := range []string{"war", "capture", "peace", "raid"} {
		if counts[kind] == 0 {
			t.Errorf("no %q events in three millennia (counts: %v)", kind, counts)
		}
	}
	if got, want := counts["peace"]+len(s.Wars()), counts["war"]; got != want {
		t.Errorf("wars don't reconcile: %d declared, %d ended + %d standing", want, counts["peace"], len(s.Wars()))
	}
	for _, w := range s.Wars() {
		if s.realmName(w.A) == "" || s.realmName(w.B) == "" {
			t.Errorf("standing war references a dead realm (%d vs %d)", w.A, w.B)
		}
	}
	checkPolity(t, *s.W)
}

// checkSimArrays asserts every per-seat parallel array stays aligned
// with W.Seats through ruins (splices) and foundings (appends), and
// that the capital index still points at the capital.
func checkSimArrays(t *testing.T, s *Sim) {
	t.Helper()
	n := len(s.W.Seats)
	for name, l := range map[string]int{
		"base": len(s.base), "temperament": len(s.temperament), "stance": len(s.stance),
		"lowStreak": len(s.lowStreak), "highStreak": len(s.highStreak), "ruinStreak": len(s.ruinStreak),
		"house": len(s.house), "houseSince": len(s.houseSince), "reignEnd": len(s.reignEnd),
	} {
		if l != n {
			t.Errorf("array %s has %d entries, %d seats", name, l, n)
		}
	}
	if s.capitalIdx >= 0 {
		if s.capitalIdx >= n || s.W.Seats[s.capitalIdx].Tier != RegionCapital {
			t.Errorf("capitalIdx %d no longer points at the capital", s.capitalIdx)
		}
	}
}

// TestSim_RiseAndFall: over three millennia halls must both fall and
// rise; fallen halls leave RegionRuin scars with no living seat, and
// raised halls stand on seat-tier cells inside their realm.
func TestSim_RiseAndFall(t *testing.T) {
	s := NewSim(42, 0)
	ruinEvents, foundingEvents := 0, 0
	for y := 0; y < 3000; y++ {
		for _, e := range s.StepYear() {
			switch e.Kind {
			case "ruin":
				ruinEvents++
			case "founding":
				foundingEvents++
			}
		}
	}
	if ruinEvents == 0 {
		t.Error("no hall fell in three millennia — dragonfire is toothless")
	}
	if foundingEvents == 0 {
		t.Error("no hall rose in three millennia — the realms are sterile")
	}
	g := gridOf(s.W.Regions)
	seatAt := make(map[[2]int64]bool, len(s.W.Seats))
	for _, st := range s.W.Seats {
		seatAt[[2]int64{st.X, st.Y}] = true
	}
	for _, r := range s.Ruins() {
		if seatAt[[2]int64{r.X, r.Y}] {
			t.Errorf("ruin %q at (%d,%d) coincides with a living seat", r.Name, r.X, r.Y)
		}
		if got := g.regionAt([2]int{int(r.X), int(r.Y)}); got != RegionRuin {
			t.Errorf("ruin %q cell is region %d, want RegionRuin", r.Name, got)
		}
	}
	checkSimArrays(t, s)
	checkPolity(t, *s.W)
}

// TestSim_HeritageLines: lines fail only by crisis (every succession
// event is a rupture), the failed house is replaced, reigns always
// extend into the future, and a crownless age still buries its lords.
func TestSim_HeritageLines(t *testing.T) {
	s := NewSim(42, 0)
	founding := make([]string, len(s.house))
	copy(founding, s.house)
	crises := 0
	for y := 0; y < 1000; y++ {
		for _, e := range s.StepYear() {
			if e.Kind != "succession" {
				continue
			}
			crises++
			if !strings.Contains(e.Text, "the line of") {
				t.Errorf("succession event without a failed line: %s", e.Text)
			}
		}
	}
	if crises == 0 {
		t.Error("no succession crises in a millennium — the lines are immortal")
	}
	changed := 0
	for i := range s.house {
		if s.house[i] != founding[i] {
			changed++
			if s.houseSince[i] == 0 {
				t.Errorf("seat %d changed house but houseSince is 0", i)
			}
		}
		if s.reignEnd[i] <= s.Year {
			t.Errorf("seat %d reign ended in the past (%d ≤ %d)", i, s.reignEnd[i], s.Year)
		}
	}
	if changed == 0 {
		t.Error("crises occurred but no house ever changed")
	}
	if changed > crises {
		t.Errorf("%d houses changed but only %d crises — houses must change only by crisis", changed, crises)
	}
}

// TestSim_PressureMatchesGenAtRest pins the gen↔sim identity: with
// every lair at activity 1, the sim's pressure formula must reproduce
// applyDragonPressure exactly — the two can never drift apart.
func TestSim_PressureMatchesGenAtRest(t *testing.T) {
	for _, seed := range []int64{1, 7, 42} {
		s := NewSim(seed, 0)
		genPressure := make([]float64, len(s.W.Seats))
		for i, st := range s.W.Seats {
			genPressure[i] = st.Pressure
		}
		s.recomputePressure() // activities untouched, all 1
		for i, st := range s.W.Seats {
			if st.Pressure != genPressure[i] {
				t.Errorf("seed %d seat %q: sim pressure %g != gen %g",
					seed, st.Name, st.Pressure, genPressure[i])
			}
		}
	}
}

// TestSim_AllLairTiersSpeak: over a millennium the chronicle must hear
// from every tier of the dragon family, not just the dens.
func TestSim_AllLairTiersSpeak(t *testing.T) {
	heard := map[string]bool{}
	for _, seed := range []int64{1, 42} {
		s := NewSim(seed, 0)
		for y := 0; y < 1000; y++ {
			for _, e := range s.StepYear() {
				if e.Kind != "lair" {
					continue
				}
				switch {
				case strings.Contains(e.Text, "dragon"):
					heard["dragon"] = true
				case strings.Contains(e.Text, "drake"):
					heard["drakes"] = true
				case strings.Contains(e.Text, "wyvern") || strings.Contains(e.Text, "rookery"):
					heard["wyverns"] = true
				}
			}
		}
	}
	for _, kind := range []string{"dragon", "drakes", "wyverns"} {
		if !heard[kind] {
			t.Errorf("no %s temper events in two millennia (heard: %v)", kind, heard)
		}
	}
}

func TestStickyStance(t *testing.T) {
	cases := []struct {
		a    float64
		cur  string
		want string
	}{
		{0.74, "sworn", "sworn"},         // within the margin: reputation holds
		{0.73, "sworn", "tributary"},     // cleared the margin: slips
		{0.76, "tributary", "tributary"}, // within the margin going up
		{0.77, "tributary", "sworn"},     // cleared it: rises
		{0.80, "autonomous", "sworn"},    // multi-band jump lands directly
		{0.10, "sworn", "autonomous"},    // and falls directly
		{0.50, "tributary", "tributary"}, // sitting on a boundary it owns
	}
	for _, c := range cases {
		if got := stickyStance(c.a, c.cur); got != c.want {
			t.Errorf("stickyStance(%.2f, %q) = %q, want %q", c.a, c.cur, got, c.want)
		}
	}
}
