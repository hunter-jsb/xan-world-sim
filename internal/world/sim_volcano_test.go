package world

import "testing"

// TestSim_EruptionsReplayDeepTime: the slice and deep time read ONE
// volcanic schedule. Find a slice with a scheduled eruption, watch it
// fire (a major event at the vent, the summit stamped onto the map,
// the vent at full heat in the shared pressure field), and confirm
// deep time records the same vent burning at the next kiloyear.
func TestSim_EruptionsReplayDeepTime(t *testing.T) {
	seed, kya := int64(-1), -1
search:
	for sd := int64(0); sd < 20; sd++ {
		_, sched := volcanoTimelineFor(sd)
		for k := 1; k <= 12; k++ {
			for _, e := range sched {
				if e.kya == k-1 {
					seed, kya = sd, k
					break search
				}
			}
		}
	}
	if seed < 0 {
		t.Fatal("no eruption inside any scanned slice — the schedule has gone silent")
	}

	s := NewSim(seed, kya)
	if len(s.eruptions) == 0 {
		t.Fatal("the timeline schedules an eruption this millennium but the sim queue is empty")
	}
	first := s.eruptions[0]
	vent := s.volcanoes[first.vi]
	s.StepMonths(first.month)

	var ev *SimEvent
	for i := range s.Log {
		if s.Log[i].Kind == "eruption" {
			ev = &s.Log[i]
			break
		}
	}
	if ev == nil {
		t.Fatal("the eruption's month passed without an eruption event")
	}
	if !ev.Major || ev.X != vent.X || ev.Y != vent.Y {
		t.Errorf("eruption event major:%v at (%d,%d), want a headline at the vent (%d,%d)",
			ev.Major, ev.X, ev.Y, vent.X, vent.Y)
	}
	if got := s.VolcanoHeat(vent.X, vent.Y); got != 1 {
		t.Errorf("vent heat after the eruption = %g, want full flame", got)
	}
	summitStamped := false
	for _, p := range s.CellPatches() {
		if p.X == vent.X && p.Y == vent.Y && p.Kind == "volcano" {
			summitStamped = true
		}
	}
	if !summitStamped {
		t.Error("the summit was never stamped volcano on the slice's map")
	}

	// Deep time's record of the same moment: at kya−1 the vent has
	// just erupted — the slice witnessed exactly what history wrote.
	w := Generate(seed, kya-1)
	recorded := false
	for _, v := range w.Volcanoes {
		if v.X == vent.X && v.Y == vent.Y && v.LastAgo == 0 {
			recorded = true
		}
	}
	if !recorded {
		t.Errorf("deep time at kya=%d does not record the eruption the slice at kya=%d witnessed", kya-1, kya)
	}

	// Determinism: the same slice replays the same fire.
	r := NewSim(seed, kya)
	r.StepMonths(first.month)
	var rev *SimEvent
	for i := range r.Log {
		if r.Log[i].Kind == "eruption" {
			rev = &r.Log[i]
			break
		}
	}
	if rev == nil || rev.Year != ev.Year || rev.Month != ev.Month || rev.Text != ev.Text {
		t.Error("replaying the slice moved or reworded the eruption")
	}
}

// TestSim_VolcanoesShareThePressureField: at rest the sim's pressure
// (lairs + vent heat) must reproduce the generator's exactly, and a
// fresh eruption must raise the pressure on halls in the vent's
// shadow — geology and dragons are one threat family.
func TestSim_VolcanoesShareThePressureField(t *testing.T) {
	s := NewSim(42, 0)
	for i, st := range s.W.Seats {
		gen := st.Pressure
		s.recomputePressure()
		if got := s.W.Seats[i].Pressure; got != gen {
			t.Fatalf("seat %q pressure at rest: sim %g vs gen %g", st.Name, got, gen)
		}
	}
	if len(s.volcanoes) == 0 {
		t.Fatal("no vents on the slice — the timeline is missing")
	}
	// Light the first vent by hand and check its shadow heats up.
	vi := 0
	v := s.volcanoes[vi]
	before := s.sitePressure(v.X+1, v.Y+1)
	s.volcanoHeat[vi] = 1
	after := s.sitePressure(v.X+1, v.Y+1)
	if want := float64(volcanoRadius-1) * 1; after < want && after <= before {
		t.Errorf("full-heat vent projects %g at its foot (was %g), want ≥ %g or at least hotter", after, before, want)
	}
}

// TestGeology_FertilityIsTotal: every rock feeds the granary model a
// sane number — the civilization stages lean on it everywhere.
func TestGeology_FertilityIsTotal(t *testing.T) {
	for rock := int64(0); rock <= 7; rock++ {
		for _, age := range []int64{0, 10, 100, 600} {
			f := SoilFertility(rock, age)
			if f < 0 || f > 1 {
				t.Errorf("SoilFertility(%d, %d) = %g, out of [0,1]", rock, age, f)
			}
		}
	}
	if SoilFertility(RockAlluvium, 0) != 1 {
		t.Error("river silt should be the best ground there is")
	}
	if SoilFertility(RockLava, 5) != 0 {
		t.Error("fresh lava feeds no one")
	}
	if SoilFertility(RockLava, 100) <= SoilFertility(RockTill, 100) {
		t.Error("weathered volcanic soil should beat glacial till — the vineyard slopes")
	}
}
