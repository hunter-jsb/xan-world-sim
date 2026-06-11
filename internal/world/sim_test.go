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
	for _, st := range s.W.Seats {
		fmt.Fprintf(&b, "seat %s a=%v p=%v r=%d\n", st.Name, st.Allegiance, st.Pressure, st.RealmID)
	}
	for _, r := range s.W.Realms {
		fmt.Fprintf(&b, "realm %d %s crown=%v\n", r.ID, r.Name, r.IsCrown)
	}
	fmt.Fprintf(&b, "territory=%d\n", len(s.W.Territory))
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
			case "secede", "swear", "dissolve", "epoch":
				if !e.Major {
					t.Errorf("year %d: %s event not Major", e.Year, e.Kind)
				}
			case "stance", "lair":
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
			if e.Kind != "lair" {
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
