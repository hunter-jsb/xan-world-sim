package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/render"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// expFixture builds a model over a synthetic strip: cradle along y=5,
// marsh at x=10..13, a den at (20,8) whose danger core covers
// x≈13..27 on the path row.
func expFixture() *model {
	m := &model{seed: 99}
	var cells []db.GetCellsInBoundsRow
	for x := int64(0); x <= 30; x++ {
		kind := "cradle"
		if x >= 10 && x <= 13 {
			kind = "marsh"
		}
		cells = append(cells, db.GetCellsInBoundsRow{X: x, Y: 5, Kind: kind, Elevation: 100})
	}
	m.data = worldData{
		cells:    cells,
		features: []db.GetNamedFeaturesInBoundsRow{{X: 20, Y: 8, Kind: "den", Name: "Test Den"}},
	}
	m.buildLookups()
	return m
}

func strip(x0, x1 int64) ([]render.PathCell, []int) {
	var path []render.PathCell
	var arrive []int
	day := 0
	for x := x0; x <= x1; x++ {
		path = append(path, render.PathCell{X: x, Y: 5})
		arrive = append(arrive, day)
		day++
	}
	return path, arrive
}

func TestBuildExpEvents_MarshAndDanger(t *testing.T) {
	m := expFixture()
	path, arrive := strip(0, 30)
	events := m.buildExpEvents(0, path, arrive)

	var titles []string
	lastDay := -1
	for _, e := range events {
		titles = append(titles, e.title)
		if e.day < lastDay {
			t.Errorf("events out of day order: %v", titles)
		}
		lastDay = e.day
		if e.roll < 0 || e.roll >= 1 {
			t.Errorf("event %q roll %g outside [0,1)", e.title, e.roll)
		}
	}
	joined := strings.Join(titles, "|")
	if !strings.Contains(joined, "fever in the camp") {
		t.Errorf("no fever event over a 4-cell marsh: %v", titles)
	}
	if !strings.Contains(joined, "wings on the horizon") {
		t.Errorf("no sighting in a den's danger core: %v", titles)
	}
	if n := strings.Count(joined, "wings on the horizon"); n != 1 {
		t.Errorf("%d sightings for one contiguous danger zone, want 1", n)
	}

	// Determinism: the same journey meets the same fortune.
	again := m.buildExpEvents(0, path, arrive)
	if fmt.Sprintf("%+v", events) != fmt.Sprintf("%+v", again) {
		t.Error("two builds of the same route diverged")
	}
}

func TestBuildExpEvents_HomeGroundSuppressed(t *testing.T) {
	m := expFixture()
	// Departing from inside the marsh: the run you start in is home
	// ground, not an encounter.
	path, arrive := strip(10, 16)
	for _, e := range m.buildExpEvents(0, path, arrive) {
		if e.title == "fever in the camp" {
			t.Error("fever event fired for the marsh the caravan started in")
		}
	}
}

// TestBuildExpEvents_WarToll: crossing into the land of a realm at
// war with the caravan's banner demands a toll — mid-sim only, once
// per hostile realm.
func TestBuildExpEvents_WarToll(t *testing.T) {
	s := world.NewSim(42, 0)
	for y := 0; y < 3000 && len(s.Wars()) == 0; y++ {
		s.StepYear()
	}
	if len(s.Wars()) == 0 {
		t.Fatal("seed 42 fought no war in three millennia — calibration moved?")
	}
	w := s.Wars()[0]

	m := expFixture()
	m.simMode, m.sim = true, s
	for x := int64(5); x <= 30; x++ {
		m.data.territory = append(m.data.territory,
			db.GetTerritoryInBoundsRow{X: x, Y: 5, RealmID: w.B, RealmName: "Enemy"})
	}
	m.buildLookups()
	path, arrive := strip(0, 30)

	tolls := 0
	for _, e := range m.buildExpEvents(w.A, path, arrive) {
		if e.title == "riders bar the road" {
			tolls++
		}
	}
	if tolls != 1 {
		t.Errorf("%d toll events crossing one hostile realm, want 1", tolls)
	}

	// The same road in deep time (no sim) carries no tolls.
	m.simMode, m.sim = false, nil
	for _, e := range m.buildExpEvents(w.A, path, arrive) {
		if e.title == "riders bar the road" {
			t.Error("toll event in deep time — wars don't exist there")
		}
	}
}

func TestResolveExpChoice(t *testing.T) {
	m := expFixture()
	ev := &expEvent{
		roll: 0.3,
		choices: []expChoice{
			{label: "risky", riskOdds: 0.5, riskDelay: 6, riskText: "mauled", safeText: "fine"},
			{label: "slow", delay: 3, safeText: "sheltered"},
			{label: "back", turnBack: true},
		},
	}
	mk := func() *expedition {
		path, arrive := strip(0, 10)
		return &expedition{phase: expRunning, path: path, arrive: arrive, day: 2, pos: 2, pending: ev}
	}

	m.exp = mk()
	m.resolveExpChoice(0) // roll 0.3 < odds 0.5 → the risk fires
	if m.exp.delay != 6 || m.status != "mauled" {
		t.Errorf("risky choice: delay %d status %q, want 6 %q", m.exp.delay, m.status, "mauled")
	}

	m.exp = mk()
	m.resolveExpChoice(1)
	if m.exp.delay != 3 || m.status != "sheltered" {
		t.Errorf("slow choice: delay %d status %q", m.exp.delay, m.status)
	}

	m.exp = mk()
	m.resolveExpChoice(2)
	if m.exp != nil {
		t.Error("turn back left the expedition standing")
	}

	// A won current can't pull the next arrival before today.
	m.exp = mk()
	m.exp.pending = &expEvent{choices: []expChoice{{label: "fast", delay: -10, safeText: "swift"}}}
	m.resolveExpChoice(0)
	if next := m.exp.arrive[m.exp.pos+1] + m.exp.delay; next < m.exp.day {
		t.Errorf("delay clamp failed: next arrival day %d before current day %d", next, m.exp.day)
	}
}
