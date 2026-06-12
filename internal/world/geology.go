package world

import (
	"math"
	"math/rand"
	"sort"
)

// Geology — the rock's own history. The structural frame (zones,
// initial heights, the erosion spin-up) is built once from the seed
// exactly as before; what's new is that the frame is the state of the
// world at geoStart, not at the present. From there Generate
// integrates a fixed geological history forward to the requested kya
// in geoStep epochs: the rift's mountains keep rising, volcanoes
// erupt on a schedule drawn from the seed alone, ice sheets scour the
// land and dump till where they let go, glacial dust settles into
// loess on the cold steppe, the crust flexes under ice load and
// rebounds late, and stream-power erosion keeps cutting with each
// epoch's own sea level as base. Because the history is a fixed
// function of the seed — never of the requested moment — scrubbing
// deep time replays the same events at the same kya every time, and
// every per-kya world remains a pure function of (seed, kya).
//
// Determinism layout: the main rng's consumption is untouched (zones,
// noise, nothing else). The volcanic schedule comes from a child rng
// keyed off the seed; all per-epoch noise is hash-keyed by
// (seed, x, y, epoch) so the integration consumes no sequential
// randomness no matter where it stops.

// Lithology — the topmost rock of every cell, persisted to
// region_cells.rock so the geological lens can draw a true geologic
// map. Numbering is mirrored in render.RockColor; keep in sync.
const (
	RockNone     int64 = 0
	RockBasement int64 = 1 // plateau shield — the old craton the rift split
	RockOrogen   int64 = 2 // folded rock of the mountain row, cliffs, doab, foothills
	RockSediment int64 = 3 // marine sediment — shelf, basin floors, old cradle fill
	RockAlluvium int64 = 4 // river-laid silt, reworked flood by flood
	RockTill     int64 = 5 // glacial till dumped by retreating ice
	RockLoess    int64 = 6 // glacial dust settled on the periglacial steppe
	RockLava     int64 = 7 // volcanic rock — fresh flows weather into basalt
)

var rockNames = map[int64]string{
	RockBasement: "basement shield",
	RockOrogen:   "orogenic rock",
	RockSediment: "marine sediment",
	RockAlluvium: "alluvium",
	RockTill:     "glacial till",
	RockLoess:    "loess",
	RockLava:     "volcanic rock",
}

// RockKind returns the display name for a lithology ("" if unknown).
func RockKind(rock int64) string { return rockNames[rock] }

// SoilFertility scores what the ground gives back to the plow, in
// [0, 1] — the one function every civilization stage reads, so the
// map's agronomy can't drift between systems. River silt and glacial
// dust feed kingdoms; weathered volcanic soil is rich (the vineyard
// slopes under the fire mountain); fresh lava feeds no one.
func SoilFertility(rock, ageAgo int64) float64 {
	switch rock {
	case RockAlluvium:
		return 1.0
	case RockLoess:
		return 0.9
	case RockLava:
		if ageAgo <= lavaFreshKa {
			return 0
		}
		return 0.8
	case RockTill:
		return 0.55
	case RockSediment:
		return 0.5
	case RockOrogen:
		return 0.25
	case RockBasement:
		return 0.2
	}
	return 0
}

// fertilityGrid maps every cell to its soil fertility — the shared
// lookup the polity stages and the slice simulation both read, so
// the same ground feeds the same kingdoms everywhere.
func (w *World) fertilityGrid() map[[2]int64]float64 {
	m := make(map[[2]int64]float64, len(w.Regions))
	for _, rc := range w.Regions {
		m[[2]int64{rc.X, rc.Y}] = SoilFertility(rc.Rock, rc.RockAge)
	}
	return m
}

// fertAround averages fertility over a cell's 3×3 neighborhood — a
// hall's granary, the land it actually farms.
func fertAround(fert map[[2]int64]float64, x, y int64) float64 {
	var sum float64
	for dy := int64(-1); dy <= 1; dy++ {
		for dx := int64(-1); dx <= 1; dx++ {
			sum += fert[[2]int64{x + dx, y + dy}]
		}
	}
	return sum / 9
}

// volcanoHeatAt is a vent's residual menace given how long ago it
// last erupted: 1 at the moment of eruption, fading to 0 over
// volcanoCoolKa. Scales the vent's pressure projection the way
// activity scales a lair's.
func volcanoHeatAt(lastAgo int64) float64 {
	f := 1 - float64(lastAgo)/volcanoCoolKa
	if f < 0 {
		return 0
	}
	return f
}

// volcanoPressureSite adapts a vent to the lair pressure formula —
// one threat family, one falloff.
func volcanoPressureSite(x, y int64) lairSite {
	return lairSite{X: x, Y: y, Kind: "volcano", Radius: volcanoRadius, Weight: 1}
}

// eruptionFrac places an eruption inside its kiloyear: the schedule
// is integer-ka (deep time's resolution), but a slice lives at month
// resolution, so each eruption gets a deterministic sub-ka moment —
// true time (e.kya + frac) ka before present. Deep time's epochs are
// unaffected (frac < 1 never crosses an epoch boundary); a slice at
// kya K replays exactly the eruptions with e.kya == K−1, at year
// (1 − frac) × 1000 of the slice. One timeline, two resolutions.
func eruptionFrac(seed int64, s volcanoSite, kya int) float64 {
	return geoHash01(seed, s.x, s.y, 1000000+kya)
}

// VolcanoInfo names a volcanic edifice on the rift shoulder. A vent
// only appears on the map once it has erupted at least once by the
// world's moment — scrub forward through deep time and new volcanoes
// are born. LastAgo and Eruptions are relative to the world's kya.
type VolcanoInfo struct {
	ID        int64
	Name      string
	X, Y      int64
	Elevation float64
	LastAgo   int64 // ka before the world's moment of the latest eruption
	Eruptions int64 // eruptions that have happened by the world's moment
}

// Geological history parameters. Rates are per ka; the loop applies
// them in geoStep chunks. Calibrated against the spin-up elevation
// profile so 600 ka of history shifts bands without breaking any
// zone's identity.
const (
	geoStart = 600 // kya — history begins; two full glacial cycles deep
	geoStep  = 5   // ka per epoch — matches the deep-time scrub stride

	// Tectonics: the rift is alive. Uplift on the mountain row runs
	// against stream-power erosion; basins sag.
	upliftMountain = 0.20 // m/ka
	upliftCliff    = 0.16
	upliftDoab     = 0.12
	upliftFoothill = 0.06
	upliftPlateau  = 0.04
	subsideBasin   = 0.02 // m/ka downward, east basin + brine floor

	// Volcanism: vents on the mountain/cliff row, each with its own
	// eruption clock.
	volcanoMin      = 4 // vents per seed: volcanoMin..volcanoMin+volcanoExtra-1
	volcanoExtra    = 3
	volcanoSep      = 12 // min Chebyshev separation between vents (cells)
	eruptGapMin     = 25 // ka between eruptions of one vent
	eruptGapMax     = 90
	coneRiseBase    = 10.0 // m the summit gains per eruption, +PerSize×size
	coneRisePerSize = 6.0
	lavaFlowBase    = 3 // flow length in cells: base + per-size×size
	lavaFlowPerSize = 3
	lavaThickness   = 6.0 // m a flow adds to each cell it crosses
	lavaFreshKa     = 15  // younger flows read as lava fields on the map

	// Glacial work: ice scours what it sits on, dumps till where it
	// retreats; the cold steppe downwind gathers loess.
	scourRate       = 0.10 // m/ka under ice
	tillBumpMax     = 4.0  // m of moraine left at a deglaciated cell
	loessRate       = 0.04 // m/ka × glacial index on the periglacial steppe
	loessMin        = 2.0  // m of dust before the surface reads as loess
	periglacialBand = 6.0  // °C above the glacier line that still gathers dust

	// Isostasy: the crust sinks under ice load and rebounds with a lag.
	isoThickPerDeg = 40.0  // m of ice per °C below the glacier line
	isoThickMax    = 800.0 // m — ice sheets top out
	isoRatio       = 0.30  // crustal depression per m of ice (ρice/ρmantle)
	isoTauKa       = 8.0   // ka — rebound relaxation time

	// Fluvial work at history pace. The spin-up's erosionK/diffusionD
	// compress megayears into 15 shaping steps; an epoch is 5 ka of
	// real time, so incision is speed-limited to a fast-but-real
	// bedrock rate and hillslope creep is slow and stays within its
	// own zone — soil creep doesn't move a rift scarp in 5 ka, and
	// unbounded diffusion would melt the cliffs and fill every lake
	// basin the spin-up carved.
	maxEpochIncision = 1.5   // m per epoch (0.3 mm/yr — fast bedrock incision)
	historyDiffusion = 0.002 // per-epoch hillslope creep factor

	// Fluvial: floodplains carrying real upstream flow are reworked
	// continually — their surface reads as young alluvium.
	alluviumMinAccum = 60 // upstream cells before a floodplain reads alluvial

	// A volcano is the geological member of the threat family: a
	// recently-active vent projects into the same pressure field the
	// dragon lairs use (lairPressureAt), cooling off over volcanoCoolKa.
	// Radius sits between a den's 12 and a rookery's 6 — the fire
	// reaches farther than wyverns, less far than a dragon's wings.
	volcanoRadius = 8
	volcanoCoolKa = 50.0

	// volcanoNameSalt keys a vent's name to its summit cell, same
	// scheme as every other named place.
	volcanoNameSalt = 7741
)

// geoHash01 mixes seed, cell, and epoch into a unit float — the
// history loop's only randomness beyond the fixed volcanic schedule.
// Hash-keyed rather than sequential so the integration's stopping
// point can never shift anyone else's draws.
func geoHash01(seed int64, x, y, epoch int) float64 {
	h := uint64(seed)*0x9E3779B97F4A7C15 ^
		uint64(int64(x))*0xBF58476D1CE4E5B9 ^
		uint64(int64(y))*0x94D049BB133111EB ^
		uint64(int64(epoch))*0xD6E8FEB86659FD93
	h ^= h >> 33
	h *= 0xFF51AFD7ED558CCD
	h ^= h >> 33
	return float64(h>>11) / float64(1<<53)
}

func baseRockForZone(z BedrockZone) int64 {
	switch z {
	case BZPlateau:
		return RockBasement
	case BZMountain, BZCliff, BZDoab, BZFoothill:
		return RockOrogen
	case BZCradle, BZBrineDeep, BZAgrariaShelf, BZAgrariaUpland, BZEastBasin:
		return RockSediment
	}
	return RockNone
}

func upliftFor(z BedrockZone) float64 {
	switch z {
	case BZMountain:
		return upliftMountain
	case BZCliff:
		return upliftCliff
	case BZDoab:
		return upliftDoab
	case BZFoothill:
		return upliftFoothill
	case BZPlateau:
		return upliftPlateau
	case BZEastBasin, BZBrineDeep:
		return -subsideBasin
	}
	return 0
}

type volcanoSite struct{ x, y int }

// eruption is one event in the fixed schedule: vent v erupts at kya
// with size 1..3.
type eruption struct {
	v    int
	kya  int
	size int
}

// drawVolcanism picks the rift's vents and lays out their entire
// eruption history from a child rng keyed off the seed alone — the
// schedule is identical no matter what kya the caller generates, so
// the same mountain blows at the same moment in every world slice.
func drawVolcanism(seed int64, zones [][]BedrockZone) ([]volcanoSite, []eruption) {
	grng := rand.New(rand.NewSource(seed*7907 + 0x9e0))
	var cands [][2]int
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if zones[y][x] == BZMountain || zones[y][x] == BZCliff {
				cands = append(cands, [2]int{x, y})
			}
		}
	}
	grng.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	n := volcanoMin + grng.Intn(volcanoExtra)
	var sites []volcanoSite
	for _, c := range cands {
		if len(sites) == n {
			break
		}
		ok := true
		for _, s := range sites {
			dx, dy := c[0]-s.x, c[1]-s.y
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if dx < volcanoSep && dy < volcanoSep {
				ok = false
				break
			}
		}
		if ok {
			sites = append(sites, volcanoSite{x: c[0], y: c[1]})
		}
	}
	var sched []eruption
	for vi := range sites {
		t := geoStart - 1 - grng.Intn(eruptGapMax)
		for t > 0 {
			sched = append(sched, eruption{v: vi, kya: t, size: 1 + grng.Intn(3)})
			t -= eruptGapMin + grng.Intn(eruptGapMax-eruptGapMin)
		}
	}
	// Oldest first; the epoch loop consumes the schedule with a cursor.
	sort.Slice(sched, func(i, j int) bool {
		if sched[i].kya != sched[j].kya {
			return sched[i].kya > sched[j].kya
		}
		return sched[i].v < sched[j].v
	})
	return sites, sched
}

// erupt applies one eruption: the cone grows at the summit and its
// flanks, and a lava flow walks steepest-descent downhill — filling
// as it goes, so big flows dam valleys and divert rivers. The flow
// quenches when it reaches the epoch's sea.
func erupt(elev [][]float64, rock [][]int64, rockAge [][]int, sites []volcanoSite, e eruption, seaLevel float64) {
	s := sites[e.v]
	rise := coneRiseBase + coneRisePerSize*float64(e.size)
	elev[s.y][s.x] += rise
	rock[s.y][s.x] = RockLava
	rockAge[s.y][s.x] = e.kya
	for _, d := range dirs8 {
		nx, ny := s.x+d[0], s.y+d[1]
		if !inBounds(nx, ny) {
			continue
		}
		elev[ny][nx] += rise / 2
		rock[ny][nx] = RockLava
		rockAge[ny][nx] = e.kya
	}
	n := lavaFlowBase + lavaFlowPerSize*e.size
	x, y := s.x, s.y
	for i := 0; i < n; i++ {
		bx, by := -1, -1
		best := elev[y][x]
		for _, d := range dirs8 {
			nx, ny := x+d[0], y+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			if elev[ny][nx] < best {
				best = elev[ny][nx]
				bx, by = nx, ny
			}
		}
		if bx < 0 {
			break // ponded — the flow stops in its own hollow
		}
		x, y = bx, by
		rock[y][x] = RockLava
		rockAge[y][x] = e.kya
		if elev[y][x] <= seaLevel {
			break // quenched — a lava delta at the shore
		}
		elev[y][x] += lavaThickness
	}
}

// iceMaskInto marks every cell the classifier would read as glacier
// under the given climate — same zone gates, same temperature line —
// so the geomorphic work and the visual truth can't disagree.
func iceMaskInto(ice [][]bool, zones [][]BedrockZone, elev [][]float64, climate ClimateState) {
	for y := 0; y < Height; y++ {
		lat := Latitude(y, DefaultLatTop, DefaultLatBottom)
		for x := 0; x < Width; x++ {
			z := zones[y][x]
			if !canGlaciate(z) || z == BZAgrariaShelf || z == BZAgrariaUpland {
				ice[y][x] = false
				continue
			}
			ice[y][x] = Temperature(lat, elev[y][x], climate) < glacierThreshold
		}
	}
}

// glacialWork is one epoch of ice geomorphology: scour under the
// sheet, till where it just let go, loess on the cold steppe beyond.
func glacialWork(seed int64, et int, elev [][]float64, rock [][]int64, rockAge [][]int, loess [][]float64, ice, prevIce [][]bool, climate ClimateState) {
	for y := 0; y < Height; y++ {
		lat := Latitude(y, DefaultLatTop, DefaultLatBottom)
		for x := 0; x < Width; x++ {
			if ice[y][x] {
				elev[y][x] -= scourRate * geoStep
				continue
			}
			if prevIce[y][x] {
				// The ice has just let go — it drops its load.
				elev[y][x] += tillBumpMax * geoHash01(seed, x, y, et)
				rock[y][x] = RockTill
				rockAge[y][x] = et
				loess[y][x] = 0
				continue
			}
			if elev[y][x] <= climate.SeaLevelDelta {
				continue // underwater gathers no dust
			}
			t := Temperature(lat, elev[y][x], climate)
			if t >= glacierThreshold && t < glacierThreshold+periglacialBand && climate.GlacialIndex > 0.2 {
				dust := loessRate * geoStep * climate.GlacialIndex
				loess[y][x] += dust
				elev[y][x] += dust
				if loess[y][x] >= loessMin && rock[y][x] != RockLoess {
					rock[y][x] = RockLoess
					rockAge[y][x] = et
				}
			}
		}
	}
}

// isostasyStep relaxes the crust toward its load: ice mass presses
// the surface down, melt lets it rise — late, so a freshly
// deglaciated coast can drown under the returning sea before the
// land comes back up.
func isostasyStep(depression [][]float64, ice [][]bool, elev [][]float64, climate ClimateState) {
	relax := 1 - math.Exp(-float64(geoStep)/isoTauKa)
	for y := 0; y < Height; y++ {
		lat := Latitude(y, DefaultLatTop, DefaultLatBottom)
		for x := 0; x < Width; x++ {
			target := 0.0
			if ice[y][x] {
				if deficit := glacierThreshold - Temperature(lat, elev[y][x], climate); deficit > 0 {
					target = math.Min(deficit*isoThickPerDeg, isoThickMax) * isoRatio
				}
			}
			d := depression[y][x] + (target-depression[y][x])*relax
			elev[y][x] -= d - depression[y][x]
			depression[y][x] = d
		}
	}
}

// landMaskInto marks the cells that erode fluvially this epoch:
// above the epoch's sea, not under ice. The exposed shelf at a low
// stand is land here — that's how the glacial rivers cut the
// channels the warm sea later drowns.
func landMaskInto(land [][]bool, elev [][]float64, ice [][]bool, seaLevel float64) {
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			land[y][x] = elev[y][x] > seaLevel && !ice[y][x]
		}
	}
}

// accumulateOver is flow accumulation with rainfall on the epoch's
// land mask (the spin-up variant rains on elevation > 0).
func accumulateOver(land [][]bool, elev [][]float64, flowDir [][]flowVec) [][]int {
	accum := make([][]int, Height)
	type lc struct {
		x, y int
		e    float64
	}
	var cells []lc
	for y := 0; y < Height; y++ {
		accum[y] = make([]int, Width)
		for x := 0; x < Width; x++ {
			if land[y][x] {
				accum[y][x] = 1
				cells = append(cells, lc{x, y, elev[y][x]})
			}
		}
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].e > cells[j].e })
	for _, c := range cells {
		d := flowDir[c.y][c.x]
		if d.dx == 0 && d.dy == 0 {
			continue
		}
		nx, ny := c.x+d.dx, c.y+d.dy
		if !inBounds(nx, ny) {
			continue
		}
		accum[ny][nx] += accum[c.y][c.x]
	}
	return accum
}

// erodeEpoch is one stream-power step graded to the epoch's sea
// level: rivers can cut the exposed shelf at a low stand, and nothing
// erodes below the water that bounds it. Incision is capped at
// maxEpochIncision — 5 ka of real time cuts meters, not the tens the
// spin-up's compressed steps move.
func erodeEpoch(elev [][]float64, land [][]bool, flowDir [][]flowVec, accum [][]int, seaLevel float64) {
	next := make([][]float64, Height)
	for y := range elev {
		next[y] = make([]float64, Width)
		copy(next[y], elev[y])
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if !land[y][x] {
				continue
			}
			d := flowDir[y][x]
			if d.dx == 0 && d.dy == 0 {
				continue
			}
			nx, ny := x+d.dx, y+d.dy
			if !inBounds(nx, ny) {
				continue
			}
			downhill := seaLevel
			if land[ny][nx] {
				downhill = elev[ny][nx]
			}
			slope := elev[y][x] - downhill
			if slope <= 0 {
				continue
			}
			cut := erosionK * math.Pow(float64(accum[y][x]), erosionM) * slope
			if cut > maxEpochIncision {
				cut = maxEpochIncision
			}
			nv := elev[y][x] - cut
			if nv < seaLevel {
				nv = seaLevel
			}
			next[y][x] = nv
		}
	}
	for y := range elev {
		copy(elev[y], next[y])
	}
}

// diffuseEpoch is one hillslope-creep step over the epoch's land —
// slow, and confined to each cell's own zone so a rift scarp stays a
// scarp and a basin stays a basin.
func diffuseEpoch(elev [][]float64, land [][]bool, zones [][]BedrockZone) {
	next := make([][]float64, Height)
	for y := range elev {
		next[y] = make([]float64, Width)
		copy(next[y], elev[y])
	}
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if !land[y][x] {
				continue
			}
			sum, count := 0.0, 0
			for _, d := range dirs8 {
				nx, ny := x+d[0], y+d[1]
				if !inBounds(nx, ny) || !land[ny][nx] || zones[ny][nx] != zones[y][x] {
					continue
				}
				sum += elev[ny][nx]
				count++
			}
			if count == 0 {
				continue
			}
			next[y][x] = elev[y][x] + historyDiffusion*(sum/float64(count)-elev[y][x])
		}
	}
	for y := range elev {
		copy(elev[y], next[y])
	}
}

// volcanoTimelineFor rebuilds the seed's volcanic timeline from the
// structural frame alone — no erosion, no history — for anyone who
// needs the schedule without paying for a full Generate. The frame
// draws consume a fresh rng exactly as generateBedrock's opening
// does, so the zones, and therefore the sites and schedule, are
// identical by construction.
func volcanoTimelineFor(seed int64) ([]volcanoSite, []eruption) {
	rng := rand.New(rand.NewSource(seed))
	mountainRow := genMountainRow(rng)
	foothillThick := genFoothillThickness(rng)
	coastX := genCoastX(rng)
	zones := make([][]BedrockZone, Height)
	for y := 0; y < Height; y++ {
		zones[y] = make([]BedrockZone, Width)
		for x := 0; x < Width; x++ {
			zones[y][x] = bedrockZone(x, y, mountainRow, foothillThick, coastX)
		}
	}
	return drawVolcanism(seed, zones)
}

// generateBedrock builds the structural frame from the main rng
// (consumption identical to the pre-history model), then integrates
// the geological history from geoStart down to the requested kya.
// Returns the bedrock at that moment, the volcano roster (only vents
// that have already erupted by then exist on the map), and the full
// site list + schedule — the one volcanic timeline that deep time
// integrates and a slice replays live.
func generateBedrock(rng *rand.Rand, seed int64, kya int) ([][]BedrockCell, []VolcanoInfo, []volcanoSite, []eruption) {
	mountainRow := genMountainRow(rng)
	foothillThick := genFoothillThickness(rng)
	coastX := genCoastX(rng)

	// Phase 1: the bedrock zone for every cell.
	zones := make([][]BedrockZone, Height)
	for y := 0; y < Height; y++ {
		zones[y] = make([]BedrockZone, Width)
		for x := 0; x < Width; x++ {
			zones[y][x] = bedrockZone(x, y, mountainRow, foothillThick, coastX)
		}
	}

	// Phase 2: initial heights — zone base + per-cell noise.
	// RNG order: y outer, x inner, every cell consumes one Float64.
	elev := make([][]float64, Height)
	for y := 0; y < Height; y++ {
		elev[y] = make([]float64, Width)
		for x := 0; x < Width; x++ {
			z := zones[y][x]
			base := roughElevationForZone(z)
			amp := zoneAmplitude(z)
			elev[y][x] = base + (rng.Float64()*2-1)*amp
		}
	}

	// Phase 3: erosion spin-up — the timeless shape of the land, the
	// state of the world at geoStart. Zone-based land, present-day
	// base level, exactly the pre-history model.
	for range erosionSteps {
		flowDir := computeFlowDirections(elev)
		accum := flowAccumulationFromElev(elev, flowDir)
		erodeStreamPower(elev, zones, flowDir, accum)
		diffuseHillslope(elev, zones)
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				if isLandZone(zones[y][x]) && elev[y][x] < 0 {
					elev[y][x] = 0
				}
			}
		}
	}

	// Phase 4: history. Lithology starts at the zone's base rock; the
	// loop overprints it event by event, most recent wins — exactly
	// what the topmost layer of a geologic map shows.
	rock := make([][]int64, Height)
	rockAge := make([][]int, Height) // kya the surface was laid (absolute)
	loess := make([][]float64, Height)
	depression := make([][]float64, Height)
	ice := make([][]bool, Height)
	prevIce := make([][]bool, Height)
	land := make([][]bool, Height)
	for y := 0; y < Height; y++ {
		rock[y] = make([]int64, Width)
		rockAge[y] = make([]int, Width)
		loess[y] = make([]float64, Width)
		depression[y] = make([]float64, Width)
		ice[y] = make([]bool, Width)
		prevIce[y] = make([]bool, Width)
		land[y] = make([]bool, Width)
		for x := 0; x < Width; x++ {
			rock[y][x] = baseRockForZone(zones[y][x])
			rockAge[y][x] = geoStart
		}
	}
	// Seed the ice state at geoStart so the first epoch doesn't read
	// a spurious world-wide deglaciation.
	iceMaskInto(prevIce, zones, elev, ClimateAt(geoStart))

	sites, sched := drawVolcanism(seed, zones)
	si := 0

	// Epochs tile [kya, geoStart): each ends at moment et and covers
	// the events of [et, et+geoStep). The last epoch ends exactly at
	// the requested kya, with exactly its climate.
	for et := kya + geoStep*((geoStart-kya-1)/geoStep); et >= kya; et -= geoStep {
		climate := ClimateAt(et)
		applyUplift(elev, zones)
		for si < len(sched) && sched[si].kya >= et {
			erupt(elev, rock, rockAge, sites, sched[si], climate.SeaLevelDelta)
			si++
		}

		iceMaskInto(ice, zones, elev, climate)
		glacialWork(seed, et, elev, rock, rockAge, loess, ice, prevIce, climate)
		isostasyStep(depression, ice, elev, climate)

		landMaskInto(land, elev, ice, climate.SeaLevelDelta)
		flowDir := computeFlowDirections(elev)
		accum := accumulateOver(land, elev, flowDir)
		erodeEpoch(elev, land, flowDir, accum, climate.SeaLevelDelta)
		diffuseEpoch(elev, land, zones)

		ice, prevIce = prevIce, ice
	}

	// Phase 5: pack. Ages become "ka before this world's moment".
	out := make([][]BedrockCell, Height)
	for y := 0; y < Height; y++ {
		out[y] = make([]BedrockCell, Width)
		for x := 0; x < Width; x++ {
			out[y][x] = BedrockCell{
				Zone:      zones[y][x],
				Elevation: elev[y][x],
				Rock:      rock[y][x],
				RockAgo:   int64(rockAge[y][x] - kya),
			}
		}
	}

	// The volcano roster at this moment.
	var vols []VolcanoInfo
	for vi, s := range sites {
		erupted, last := 0, -1
		for _, e := range sched {
			if e.v == vi && e.kya >= kya {
				erupted++
				if last == -1 || e.kya < last {
					last = e.kya
				}
			}
		}
		if erupted == 0 {
			continue // not yet born — still an ordinary peak
		}
		vols = append(vols, VolcanoInfo{
			ID:        int64(len(vols) + 1),
			Name:      generateName(nameSeedForCell(seed, int64(s.x), int64(s.y)) + volcanoNameSalt),
			X:         int64(s.x),
			Y:         int64(s.y),
			Elevation: elev[s.y][s.x],
			LastAgo:   int64(last - kya),
			Eruptions: int64(erupted),
		})
	}
	return out, vols, sites, sched
}

func applyUplift(elev [][]float64, zones [][]BedrockZone) {
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			elev[y][x] += upliftFor(zones[y][x]) * geoStep
		}
	}
}
