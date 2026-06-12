package render

import "github.com/hunterjsb/xan-world-sim/internal/db"

// Lenses — alternative colorings of the same grid. Every lens is one
// cellColorFn over the shared builder: terrain (the default kind
// ramps), political (realm tint), climate (a temperature ramp fed by
// the world's own Temperature function), geological (a true geologic
// map — the topmost lithology laid down by the seed's own history of
// fire, ice, and rivers), and ecological (life zones: vegetation
// classes pop, civilization fades, lairs glow as apex fauna).
// Glyphs never change — a lens recolors the world, it doesn't
// redraw it.

// BuildGridBufWith is the open variant of BuildGridBuf: callers
// supply the lens's color function.
func BuildGridBufWith(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, colorOf cellColorFn) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY, colorOf)
}

// climateBand maps a temperature (°C) to the lens ramp: deep frozen
// blues through temperate greens to scorched reds. Bounds chosen
// around the world's own thresholds (glaciation near −2°C, the warm
// cradle above ~15°C).
func climateBand(t float64) (string, bool) {
	switch {
	case t < -10:
		return "27", true // deep frozen blue
	case t < -2:
		return "33", false // glacial blue (the ice line lives here)
	case t < 2:
		return "51", false // cyan — freeze's edge
	case t < 8:
		return "121", false // cool pale green
	case t < 14:
		return "112", false // temperate green
	case t < 20:
		return "184", false // warm yellow
	case t < 26:
		return "214", false // hot orange
	default:
		return "196", true // scorched red
	}
}

// BuildClimateGridBuf colors every cell by temperature. tempAt is the
// caller's bridge to the world's Temperature function, so the lens
// can never disagree with the classifier that built the map.
func BuildClimateGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, tempAt func(x, y int64, elev float64) float64) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return climateBand(tempAt(x, y, elev))
		})
}

// RockColor is the geological lens band: the topmost lithology,
// colored like a geologic map — shield pink, orogen umber, sediment
// slate, alluvium yellow, till olive, loess gold, basalt
// purple-black with fresh flows glowing. Bold marks ground the ice
// or the fire left within the last ~20 ka. Rock numbering mirrors
// world/geology.go's Rock* constants.
//
// Unlike the other lenses there is no flat water tint: the rock
// doesn't care about the sea, so the lens X-rays straight through it
// (and through the ice sheets) — glyphs still carry the water.
func RockColor(rock, ageAgo int64) (string, bool) {
	switch rock {
	case 1:
		return "168", false // basement shield — geologic-map pink
	case 2:
		return "95", false // orogenic rock — folded umber
	case 3:
		return "67", false // marine sediment — slate blue
	case 4:
		return "185", false // alluvium — quaternary yellow ribbons
	case 5:
		return "108", ageAgo <= 20 // glacial till — olive drift
	case 6:
		return "222", ageAgo <= 20 // loess — pale gold dust
	case 7:
		if ageAgo <= 15 {
			return "196", true // fresh lava — still glowing
		}
		return "54", false // old basalt — purple-black
	}
	return "240", false // unsurveyed
}

// BuildGeoGridBuf is the geological lens: lithology and its age, with
// the caller bridging to its row data.
func BuildGeoGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, rockAt func(x, y int64) (int64, int64)) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return RockColor(rockAt(x, y))
		})
}

// ecoClassColor groups every kind into a life zone. Civilization
// (seats, roads' endpoints, ruins) fades to gray so the living world
// reads through it; the dragon family glows — apex fauna is the
// ecology's headline.
func ecoClassColor(kind string) (string, bool) {
	switch kind {
	case "sea_brine", "sea_eastern", "drowned":
		return "24", false // open water
	case "lake":
		return "31", false // fresh water
	case "glacier":
		return "195", false // ice
	case "mountain", "cliff", "plateau", "pass", "unknown", "volcano":
		return "245", false // barren rock
	case "lava":
		return "238", false // sterile ground — succession hasn't started
	case "tundra":
		return "144", false // cold steppe
	case "cradle", "doab", "agraria_upland":
		return "107", false // open grass
	case "agraria":
		return "143", false // tilled ground
	case "forest":
		return "28", true // deep wood
	case "marsh":
		return "37", false // wetland
	case "foothill":
		return "101", false // scrub hills
	case "den", "nest", "rookery":
		return "160", true // apex fauna — the ecology's headline
	default:
		// seats of every tier, the capital, ruins: civilization,
		// dimmed so the life zones read through it.
		return "242", false
	}
}

// BuildEcoGridBuf is the ecological lens.
func BuildEcoGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return ecoClassColor(kind)
		})
}

// DrainageColor is the hydrology lens band: how much water passes
// through a cell, log-scaled — quiet land in dry earth tones, then
// brighter blues as creeks gather into trunks. Open water reads flat
// deep blue (the sea is where drainage goes to die).
func DrainageColor(kind string, drainage int64) (string, bool) {
	if waterOrIceKind(kind) {
		return "17", false
	}
	switch {
	case drainage < 4:
		return "237", false // dry interfluve
	case drainage < 16:
		return "60", false // damp ground
	case drainage < 64:
		return "67", false // creek
	case drainage < 256:
		return "39", false // stream
	case drainage < 1024:
		return "45", true // river
	default:
		return "51", true // trunk — the cradle's Mississippi
	}
}

// BuildHydroGridBuf is the hydrology lens: drainage per cell, with
// the caller bridging to its row data.
func BuildHydroGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, drainAt func(x, y int64) int64) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return DrainageColor(kind, drainAt(x, y))
		})
}

// DangerColor is the danger lens band: lair raid heat as the
// expedition pathfinder prices it (live activity included in sim).
// Safe ground stays near-black; the ramp climbs through ember to
// open flame around den cores.
func DangerColor(kind string, danger int) (string, bool) {
	if waterOrIceKind(kind) {
		return "236", false
	}
	switch {
	case danger <= 0:
		return "238", false // safe
	case danger < 6:
		return "58", false // uneasy
	case danger < 12:
		return "94", false // raided
	case danger < 21:
		return "130", false // dangerous
	case danger < 33:
		return "166", true // deadly
	default:
		return "196", true // a lair's own doorstep
	}
}

// BuildDangerGridBuf is the danger lens.
func BuildDangerGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64, dangerAt func(x, y int64) int) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return DangerColor(kind, dangerAt(x, y))
		})
}
