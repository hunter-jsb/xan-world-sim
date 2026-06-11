package main

import "strings"

// The help browser: H opens a topic menu and each feature gets an
// explorable page, all through the one modal popup primitive. Pages
// are declarative data here; the menu remembers where you were.

// helpTopic is one explorable page.
type helpTopic struct {
	title string
	body  []string
}

// helpTopics builds the browsable pages. The legend page reads the
// renderer's own legend so it can never drift from the map.
func (m *model) helpTopics() []helpTopic {
	return []helpTopic{
		{"the map — legend & glyphs", append(strings.Split(m.legend, "\n"),
			"",
			"rivers flow as > < v \\ / arrows toward their mouths; roads are · dots",
			"ruined halls render as ash-gray h — sacked in a simulation, hoppable like any place",
			"the political view (p) tints claimed land by realm; unclaimed wilds go dim,",
			"contested marchland pale gray, and water keeps its color to anchor the map",
			"",
			"the map annotates itself: named places tooltip beside the cursor, notices",
			"toast in the top-right corner, and fresh headlines tag the cells they hit")},

		{"time — deep time & the slice", []string{
			"the world is one continuous function of (seed, kya) — kya is kiloyears before present.",
			"",
			"deep time scrubs BETWEEN worlds: each kya is an independent equilibrium snapshot,",
			"politics as the geography would settle it. ice retreats, rivers grow, realms form.",
			"",
			"S pins the current kya as a SLICE and runs years INSIDE it: geography holds still,",
			"politics comes alive. the brackets drive time in both modes —",
			"  deep time:  ] [ step ±5 ka    } { step ±25 ka    e jumps now ↔ LGM",
			"  in a slice: ] [ speed up/down  } { snap fastest/slowest  space pauses",
			"",
			"while a slice runs (or a caravan is afield) deep time is pinned: scrubbing would",
			"dissolve the world under it. S leaves; re-entering replays the same history.",
		}},

		{"simulation — politics year by year", []string{
			"each year, in order: lairs stir, courts drift, generations turn, bonds break,",
			"halls fall and rise, wars run, borders re-settle.",
			"",
			"allegiance drifts toward what geography, temperament, and dragon pressure allow.",
			"stances (sworn / tributary / nominal / autonomous) shift with hysteresis —",
			"reputations change slower than moods. sustained collapse means secession;",
			"sustained loyalty means swearing in. every hall has a house; lines fail (12%),",
			"and a failed line on the throne ripples doubt through every sworn hall.",
			"",
			"nothing pauses the years: headlines (secessions, wars, sackings, foundings)",
			"take the status line under a ⚑ and ping the map red where they happened;",
			"g jumps to the latest news. L opens the chronicle — every entry opens a page",
			"with its impact and the thread of causes behind it (a ruin points at the",
			"dragon's stir, a war at the grievance that seeded it — follow the thread back).",
		}},

		{"realms & war", []string{
			"the crown is the downstream heartland power; its reach is the river network",
			"(allegiance = 1/(1 + distance/λ) by the crown-courier metric: rivers 1, roads 2).",
			"independent halls cluster into leagues by valley distance.",
			"",
			"grievance is heat between realms — secessions and captures pour it in, shared",
			"borders add friction, time bleeds it off. enough heat means war: raids burn the",
			"front hall's fields (a crown that cannot protect its halls loses them), and at",
			"full score the front hall falls to the winner. peace comes by capture or exhaustion.",
			"",
			"borders are alive: conviction stretches each hall's reach, war fortune pushes the",
			"front, and near-tied ground renders as contested marchland — pale no-man's gray.",
		}},

		{"dragons & lairs", []string{
			"three tiers, all placed at local peaks: dragon dens D on mountains (raid radius 12),",
			"drake nests d on foothills (8), wyvern rookeries w on cliffs (6).",
			"",
			"every lair projects pressure onto nearby halls — the strongest single threat,",
			"weighted by tier. pressure taxes allegiance: self-sufficiency breeds independence.",
			"",
			"in a slice each lair's activity wanders: rampant dragons double their reach,",
			"dormant ones free the roads. only dragonfire can sack a hall outright —",
			"drakes harass, never raze — and the marches that live against the mountains",
			"are hardened for exactly that life.",
			"",
			"lair danger also prices expedition routes: the danger map scales with live activity.",
		}},

		{"expeditions & the road", []string{
			"s proposes a journey: the settlement nearest the cursor (by travel cost, danger",
			"included) offers a caravan. depart, and it marches on its own day clock —",
			"river 1 day/cell by boat, open land 4, marsh 8.",
			"",
			"the road writes its hazards at departure, and the same road always meets the",
			"same fortune: lair shadows (press on / shelter / turn back), marsh fever",
			"(rest / march), swift currents (ride for won days), and — mid-simulation —",
			"toll riders in the land of a realm at war with the caravan's banner.",
			"",
			"the day clock holds under any popup. s abandons; arrival concludes.",
			"deep time stays pinned while a caravan is afield — it lives in one frozen moment.",
		}},

		{"keys", []string{
			"hjkl / arrows   move the cursor          enter      inspect cell (dossier + actions)",
			"w / b           hop next / prev place    o          list all places, jump from the list",
			"] [ } {         drive time (see: time)   e          jump now ↔ LGM (deep time)",
			"r               reroll the seed          p          toggle political view",
			"S               enter / leave the slice  space      pause the years (in a slice)",
			"L               the chronicle (slice)    g          jump to the latest news (slice)",
			"s               expedition to cursor / abandon one afield",
			"H               this browser             q / esc    quit / close / back out",
			"",
			"in popups: ↑↓ or jk select, enter chooses, esc closes. space never chooses —",
			"it would race the simulation's event popups when pausing.",
		}},
	}
}

// openHelpPopup is the topic menu; it remembers the last page read.
func (m *model) openHelpPopup() {
	topics := m.helpTopics()
	opts := make([]popupOption, 0, len(topics)+1)
	for i, t := range topics {
		opts = append(opts, popupOption{label: t.title, action: popHelpTopic, arg: i})
	}
	opts = append(opts, popupOption{label: "Close", action: popClose})
	sel := m.helpSel
	if sel < 0 || sel >= len(topics) {
		sel = 0
	}
	m.popup = &popupState{
		title: "the cradle — help",
		body:  []string{dimStyle.Render("pick a topic; every page links back here")},
		opts:  opts,
		sel:   sel,
	}
	m.mapStr = m.buildMap()
}

// openHelpTopic shows one page with a way back to the menu.
func (m *model) openHelpTopic(i int) {
	topics := m.helpTopics()
	if i < 0 || i >= len(topics) {
		return
	}
	m.helpSel = i
	t := topics[i]
	body := make([]string, len(t.body))
	for j, l := range t.body {
		body[j] = dimStyle.Render(l)
	}
	m.popup = &popupState{
		title: t.title,
		body:  body,
		opts: []popupOption{
			{label: "Back to topics", action: popHelpMenu},
			{label: "Close", action: popClose},
		},
	}
	m.mapStr = m.buildMap()
}
