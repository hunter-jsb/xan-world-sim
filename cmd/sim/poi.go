package main

import (
	"fmt"
	"sort"

	"github.com/hunterjsb/xan-world-sim/internal/render"
)

// poi is one point of interest — a named place the cursor can hop to:
// seats of every tier and named features (lakes, passes, lairs).
type poi struct {
	X, Y int64
	Name string
	Kind string
}

// buildPOIs collects and orders the world's named places in (y, x)
// scan order — the same canonical order the generator emits things in,
// so w/b hop reads like text: left to right, top to bottom.
func buildPOIs(data worldData) []poi {
	pois := make([]poi, 0, len(data.seats)+len(data.features))
	for _, s := range data.seats {
		pois = append(pois, poi{s.X, s.Y, s.Name, s.Tier})
	}
	for _, f := range data.features {
		pois = append(pois, poi{f.X, f.Y, f.Name, f.Kind})
	}
	sort.Slice(pois, func(i, j int) bool {
		if pois[i].Y != pois[j].Y {
			return pois[i].Y < pois[j].Y
		}
		return pois[i].X < pois[j].X
	})
	return pois
}

// poiAfter returns the index of the first POI strictly after (x, y) in
// scan order, wrapping to the first POI; -1 if there are none.
func poiAfter(pois []poi, x, y int64) int {
	for i, p := range pois {
		if p.Y > y || (p.Y == y && p.X > x) {
			return i
		}
	}
	if len(pois) == 0 {
		return -1
	}
	return 0
}

// poiBefore returns the index of the last POI strictly before (x, y)
// in scan order, wrapping to the last POI; -1 if there are none.
func poiBefore(pois []poi, x, y int64) int {
	for i := len(pois) - 1; i >= 0; i-- {
		p := pois[i]
		if p.Y < y || (p.Y == y && p.X < x) {
			return i
		}
	}
	if len(pois) == 0 {
		return -1
	}
	return len(pois) - 1
}

// jumpPOI moves the cursor to the next (dir > 0) or previous (dir < 0)
// point of interest.
func (m *model) jumpPOI(dir int) {
	var i int
	if dir > 0 {
		i = poiAfter(m.pois, m.curX, m.curY)
	} else {
		i = poiBefore(m.pois, m.curX, m.curY)
	}
	if i < 0 {
		m.status = "no points of interest in this age"
		return
	}
	p := m.pois[i]
	m.curX, m.curY = p.X, p.Y
	m.status = fmt.Sprintf("→ %s (%s)", p.Name, render.KindDisplay(p.Kind))
	m.mapStr = m.buildMap()
}

// openPOIPopup lists every point of interest in hop order; choosing
// one jumps the cursor there. Selection opens at the POI w would hop
// to, so the list is oriented around where you are.
func (m *model) openPOIPopup() {
	if len(m.pois) == 0 {
		m.status = "no points of interest in this age"
		return
	}
	opts := make([]popupOption, len(m.pois))
	for i, p := range m.pois {
		opts[i] = popupOption{
			label:  fmt.Sprintf("%-14s %-15s (%d,%d)", p.Name, render.KindDisplay(p.Kind), p.X, p.Y),
			action: popJumpPOI,
			arg:    i,
		}
	}
	sel := poiAfter(m.pois, m.curX, m.curY)
	if sel < 0 {
		sel = 0
	}
	m.popup = &popupState{
		title: fmt.Sprintf("points of interest — %d places, in w/b order", len(m.pois)),
		opts:  opts,
		sel:   sel,
	}
	m.mapStr = m.buildMap()
}
