package main

import (
	"fmt"

	"github.com/hunterjsb/xan-world-sim/internal/world"
)

// The annals browser: in deep time, L reads the sealed ages — the
// written history of the millennia already simulated. (Inside a
// slice, L is the live chronicle as ever.)

// annalsPageMax bounds how many entries one age's page shows — the
// annals are excerpts, not the whole chronicle.
const annalsPageMax = 16

// openAnnalsPopup lists the sealed ages, newest first.
func (m *model) openAnnalsPopup() {
	if len(m.chain) == 0 {
		m.status = "no ages sealed yet — run a slice past its epoch and seal it, or press n"
		return
	}
	opts := make([]popupOption, 0, len(m.chain)+1)
	for i := len(m.chain) - 1; i >= 0; i-- {
		f := m.chain[i]
		opts = append(opts, popupOption{
			label: fmt.Sprintf("the %s age — sealed into %dkya (%d entries)",
				world.AgeOrdinal(f.Age), f.Kya-1, len(f.Annals)),
			action: popAnnalsAge, arg: i,
		})
	}
	opts = append(opts, popupOption{label: "Close", action: popClose})
	m.popup = &popupState{
		title: fmt.Sprintf("the annals — %d sealed age(s)", len(m.chain)),
		body:  []string{dimStyle.Render("what the world remembers of the ages already lived")},
		opts:  opts,
	}
	m.mapStr = m.buildMap()
}

// openAnnalsAgePopup shows one sealed age's majors.
func (m *model) openAnnalsAgePopup(i int) {
	if i < 0 || i >= len(m.chain) {
		return
	}
	f := m.chain[i]
	var body []string
	shown := len(f.Annals)
	if shown > annalsPageMax {
		shown = annalsPageMax
	}
	for _, e := range f.Annals[:shown] {
		body = append(body, dimStyle.Render(fmt.Sprintf("y%4d  %s", e.Year, e.Text)))
	}
	if rest := len(f.Annals) - shown; rest > 0 {
		body = append(body, dimStyle.Render(fmt.Sprintf("…and %d more entries the excerpt leaves out", rest)))
	}
	if len(f.Annals) == 0 {
		body = []string{dimStyle.Render("a quiet age — the scribes recorded nothing of note")}
	}
	m.popup = &popupState{
		title: fmt.Sprintf("annals of the %s age", world.AgeOrdinal(f.Age)),
		body:  body,
		opts: []popupOption{
			{label: "Back to the ages", action: popAnnals},
			{label: "Close", action: popClose},
		},
	}
	m.mapStr = m.buildMap()
}
