package main

import "strings"

// The help browser: H opens a tree of pages, all through the one
// modal popup primitive. The root reads top to bottom in order of
// need — keys first, then the legend, then the systems folder (how
// the machine works) and the lore folder (what the world remembers).
// Folders open as submenus; every page links back up.

// helpNode is one entry in the tree: a page (body) or a folder
// (children). Folders show with a ▸ in menus.
type helpNode struct {
	title    string
	body     []string
	children []helpNode
}

// helpTree builds the browsable tree. The legend page reads the
// renderer's own legend so it can never drift from the map.
func (m *model) helpTree() []helpNode {
	return []helpNode{
		{title: "keys", body: []string{
			"hjkl / arrows   move the cursor          enter      inspect cell (dossier + actions)",
			"w / b           hop next / prev place    o          list all places, jump from the list",
			"] [ } {         drive time (see: time)   e          jump now ↔ LGM (deep time)",
			"r               reroll the seed          p          cycle lenses (see: the map)",
			"S               enter / leave the slice  space      pause the clock (in a slice)",
			"L               the chronicle (slice)    g          jump to the latest news (slice)",
			"s               expedition to cursor / abandon one afield",
			"H               this browser             q / esc    quit / close / back out",
			"",
			"in popups: ↑↓ or jk select, enter chooses, esc closes. space never chooses —",
			"it would race the simulation's event popups when pausing.",
		}},

		{title: "the map — legend & glyphs", body: append(strings.Split(m.legend, "\n"),
			"",
			"rivers flow as > < v \\ / arrows toward their mouths; roads are · dots",
			"ruined halls render as ash-gray h — sacked in a simulation, hoppable like any place",
			"volcanoes are bold red ! on the rift shoulder; a fresh flow is an ember-dark & field",
			"p cycles seven LENSES — colorings of the same map (glyphs never change):",
			"  terrain     the land as it lives, kind-colored and elevation-shaded",
			"  political   claimed land tinted by realm; wilds dim, contested marchland pale",
			"  climate     a temperature ramp, frozen blue to scorched red — watch the ice line",
			"  geological  a true geologic map: the topmost rock and when it was laid —",
			"              shield pink, till olive, loess gold, lava glowing — X-rays sea and ice",
			"  ecological  life zones: forests and wetlands pop, civilization fades to gray,",
			"              and the dragon family glows as apex fauna",
			"  hydrology   where the water goes: drainage gathering from dry interfluve",
			"              through creeks into the trunk rivers, log-scaled blues",
			"  danger      lair raid heat as the caravans price it — live in a slice,",
			"              a rampant dragon's reach burns wider",
			"",
			"the map annotates itself: named places tooltip beside the cursor, notices",
			"toast in the top-right corner, and every event tags the cell it hit for a",
			"few real seconds — pause (space) and tags hold as long as you read")},

		{title: "systems", children: []helpNode{
			{title: "time — deep time & the slice", body: []string{
				"the world is one continuous function of (seed, kya) — kya is kiloyears before present.",
				"",
				"deep time scrubs BETWEEN worlds: each kya is an independent equilibrium snapshot,",
				"politics as the geography would settle it. ice retreats, rivers grow, realms form.",
				"",
				"S pins the current kya as a SLICE and runs time INSIDE it: geography holds still,",
				"politics comes alive. the brackets drive time in both modes —",
				"  deep time:  ] [ step ±5 ka    } { step ±25 ka    e jumps now ↔ LGM",
				"  in a slice: ] [ speed up/down  } { snap moon/8×  space pauses",
				"the clock runs moon by moon at the slow end — a month per tick, then seasons,",
				"then whole years; the engine itself always steps monthly.",
				"",
				"while a slice runs (or a caravan is afield) deep time is pinned: scrubbing would",
				"dissolve the world under it. S leaves; re-entering replays the same history.",
			}},
			{title: "simulation — politics month by month", body: []string{
				"each month, in order: lairs stir, courts drift, generations turn, bonds break,",
				"halls fall and rise, wars run, borders re-settle.",
				"",
				"allegiance drifts toward what geography, temperament, and dragon pressure allow.",
				"stances (sworn / tributary / nominal / autonomous) shift with hysteresis —",
				"reputations change slower than moods. sustained collapse means secession;",
				"sustained loyalty means swearing in. every hall has a house; lines fail (12%),",
				"and a failed line on the throne ripples doubt through every sworn hall.",
				"",
				"nothing pauses the clock: headlines (secessions, wars, sackings, foundings)",
				"take the status line under a ⚑ and ping the map red where they happened;",
				"g jumps to the latest news. L opens the chronicle — every entry opens a page",
				"with its impact and the thread of causes behind it (a ruin points at the",
				"dragon's stir, a war at the grievance that seeded it — follow the thread back).",
			}},
			{title: "realms & war", body: []string{
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
			{title: "rock & fire — the living geology", body: []string{
				"the rock has its own history, fixed by the seed: from 600 kya the rift's",
				"mountains rise against the rivers that cut them, volcanoes erupt on their",
				"own clocks, and two full ice ages advance and retreat. scrub deep time and",
				"the same mountain blows at the same moment, every time — and a slice whose",
				"millennium holds a scheduled eruption WATCHES it: the peak splits, the flow",
				"buries halls and lairs in its path, ash thins every nearby court's loyalty,",
				"and the vent burns in the danger map until it cools. one timeline, two views.",
				"",
				"each eruption grows a cone (bold red !) and pours a flow downhill — fresh",
				"lava (&) is razor wasteland no road crosses, but within ~15 ka it weathers",
				"back into the land it buried. the rock remembers: the geological lens shows",
				"old basalt long after the surface heals.",
				"",
				"ice works slower. sheets scour the ground they ride, dump till where they",
				"let go, and dust the cold steppe beyond with loess — the cradle is fertile",
				"because the glaciers ground the mountains into flour. under the ice the",
				"crust itself sinks, and rebounds late: a freshly thawed coast can drown",
				"under the returning sea before the land comes back up.",
				"",
				"and the rock writes the politics: soil fertility (river silt, loess, old",
				"volcanic ground) feeds capital choice, founding sites, and realm strength",
				"in war — grain feeds armies. a hot vent presses on nearby halls exactly",
				"as a dragon does: one threat family, one pressure field.",
			}},
			{title: "dragons & lairs", body: []string{
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
			{title: "fate — the sealed ages", body: []string{
				"a slice spans exactly one step of deep time (1000 years = 1 ka). once it has",
				"run its course, leaving it (S) offers to SEAL THE AGE: what stands and what",
				"fell becomes the world at the next kya — deep time stepped forward from",
				"within the simulation.",
				"",
				"what survives a millennium: places, names, and stories. the halls still",
				"standing fold into the next world (an old capital returns as a great river",
				"hall — the crown is sworn anew each age, and may well choose it again);",
				"every hall that fell leaves a TELL — an ash-gray h whose tooltip carries",
				"the old chronicle line; ruling houses keep their names across the dawn.",
				"stances, grievances, and wars wash out — every age swears its oaths anew.",
				"",
				"the ground reconciles the record: a tell the sea has drowned, the ice has",
				"taken, or the lava has buried lives on only in the annals.",
				"",
				"sealing is choosing a future: it replaces any age previously sealed at or",
				"after that moment. each seed keeps its own chain (the header counts the",
				"sealed ages); the chain survives restarts, and the same slice always seals",
				"the same fate — watched or not, history is deterministic.",
			}},
			{title: "expeditions & the road", body: []string{
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
		}},

		{title: "lore", children: []helpNode{
			{title: "the deep history", body: []string{
				"humankind woke ~250,000 years ago in caves on the frozen northern plateau —",
				"the ice was the cradle, because the warm lowlands belonged to dragons and",
				"megafauna no early hunter could contest. the cold was a refuge, not a curse.",
				"",
				"two peoples diverged: Northerners, mammoth-hunters of the plateau, and",
				"Coastals, who settled the receded shores and learned to farm. through every",
				"long climate cycle they traded, sundered, raided, and reunited — warming",
				"breaks the routes, scarcity drives one-way plunder voyages, cooling knits",
				"the world back together. the pattern repeated for two hundred millennia.",
				"",
				"the Melt (~10–20 kya) carved the mountain barrier brutal and built the",
				"fertile cradle from glacial outwash. its settlers — mixed Northern and",
				"Coastal stock — followed the new rivers south and east, and only eons later",
				"drifted back upstream: the old homeland's mountains are now the frontier.",
				"",
				"now the world turns cold again. seas recede; drowned ruins of older warm",
				"ages are surfacing. the deep past is becoming reachable. this is the Turning.",
			}},
			{title: "the cradle & its peoples", body: []string{
				"the cradle is the new fertile land south of the mountain barrier — glacial",
				"outwash threaded by young rivers, the heart of post-Melt civilization.",
				"",
				"around it: the high plateau of the old kingdom in the frozen north, where",
				"pure Northerners still hold the first caves; Agraria in the northwest, the",
				"drowned ancestral coast that resurfaces as the seas fall; the ancient salty",
				"Brine to the west; the young diluted Eastern Sea, whose gentler waters drew",
				"the first cradle settlements; and unknown lands beyond it.",
				"",
				"the heartland grew downstream — river-fed, populous, calm. the north is",
				"frontier: salmon-lords on the upper rivers, the garden valley of the Doab,",
				"and halls that answer to the mountains before they answer to any crown.",
			}},
			{title: "the dragon family", body: []string{
				"three winged predators shape the world, each to its own scale of fear:",
				"",
				"the DRAGON is the apex — six-limbed, fire-breathed, often wise enough to",
				"speak, scheme, and hold a grudge across decades. rare; one arriving over a",
				"town is an extinction-level event. they den high in mountain caves.",
				"",
				"the DRAKE is the everyday menace — smaller, bestial, what northern defenses",
				"are actually built against. ballista towers and stone-roofed halls exist",
				"because wooden longhouses cook the first time a drake breathes on them.",
				"",
				"the WYVERN is the lesser raider — bipedal, poison-tailed, colonial on the",
				"cliffs. a flock is more nuisance than catastrophe, unless you are a caravan.",
				"",
				"northerners call all three 'drakes', loosely. 'dragon' is reserved — everyone",
				"knows when that is what you mean.",
			}},
			{title: "the crown & the marches", body: []string{
				"geography writes the politics. the wealthy downstream heartland — grain,",
				"trade, the great river — crowns a capital where the waters converge, and",
				"the river is the crown's reach: what a courier can travel, a crown can hold.",
				"",
				"upstream is another world. the marches sit against the mountain wall under",
				"constant dragon pressure — battle-hardened, independent-minded, bound to the",
				"crown by duty more than love: 'we are the wall.' headwater holds keep the",
				"sacred sources; outholds scratch a living off the grid; the reaches are",
				"essentially autonomous in practice, too far for any writ to matter.",
				"",
				"defense demands self-sufficiency, and self-sufficiency breeds independence —",
				"so the same mountains that shield the cradle keep trying to break it apart.",
			}},
		}},
	}
}

// helpNodeAt walks the tree to the node a path of child indexes
// names; an empty path is the (virtual) root.
func (m *model) helpNodeAt(path []int) (helpNode, bool) {
	node := helpNode{title: "the cradle — help", children: m.helpTree()}
	for _, i := range path {
		if i < 0 || i >= len(node.children) {
			return helpNode{}, false
		}
		node = node.children[i]
	}
	return node, true
}

// openHelpPopup opens the help tree at the root.
func (m *model) openHelpPopup() {
	m.helpPath = nil
	m.openHelpMenu(m.helpSel)
}

// openHelpMenu lists the children of the node at m.helpPath with the
// selection on sel. Folders carry a ▸; the root closes, deeper
// levels go back up.
func (m *model) openHelpMenu(sel int) {
	node, ok := m.helpNodeAt(m.helpPath)
	if !ok || len(node.children) == 0 {
		return
	}
	opts := make([]popupOption, 0, len(node.children)+1)
	for i, c := range node.children {
		label := c.title
		if len(c.children) > 0 {
			label += "  ▸"
		}
		opts = append(opts, popupOption{label: label, action: popHelpTopic, arg: i})
	}
	if len(m.helpPath) == 0 {
		opts = append(opts, popupOption{label: "Close", action: popClose})
	} else {
		opts = append(opts, popupOption{label: "Back", action: popHelpUp})
	}
	if sel < 0 || sel >= len(opts) {
		sel = 0
	}
	m.popup = &popupState{
		title: node.title,
		body:  []string{dimStyle.Render("pick an entry; every page links back")},
		opts:  opts,
		sel:   sel,
	}
	m.mapStr = m.buildMap()
}

// openHelpEntry descends to child i of the current menu: folders open
// as submenus, pages render with a way back.
func (m *model) openHelpEntry(i int) {
	node, ok := m.helpNodeAt(append(append([]int{}, m.helpPath...), i))
	if !ok {
		return
	}
	if len(m.helpPath) == 0 {
		m.helpSel = i
	}
	if len(node.children) > 0 {
		m.helpPath = append(m.helpPath, i)
		m.openHelpMenu(0)
		return
	}
	m.helpEntrySel = i
	body := make([]string, len(node.body))
	for j, l := range node.body {
		body[j] = dimStyle.Render(l)
	}
	m.popup = &popupState{
		title: node.title,
		body:  body,
		opts: []popupOption{
			{label: "Back", action: popHelpMenu},
			{label: "Close", action: popClose},
		},
	}
	m.mapStr = m.buildMap()
}

// helpUp pops one folder level and reopens that menu with the folder
// we came from selected.
func (m *model) helpUp() {
	if len(m.helpPath) == 0 {
		m.openHelpMenu(m.helpSel)
		return
	}
	last := m.helpPath[len(m.helpPath)-1]
	m.helpPath = m.helpPath[:len(m.helpPath)-1]
	m.openHelpMenu(last)
}
