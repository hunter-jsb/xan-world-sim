package render

import "github.com/hunterjsb/xan-world-sim/internal/db"

// Lenses — alternative colorings of the same grid. Every lens is one
// cellColorFn over the shared builder: terrain (the default kind
// ramps), political (realm tint), climate (a temperature ramp fed by
// the world's own Temperature function), geological (hypsometric
// bedrock elevation — the rift and shelf story, biomes ignored), and
// ecological (life zones: vegetation classes pop, civilization fades,
// lairs glow as apex fauna). Glyphs never change — a lens recolors
// the world, it doesn't redraw it.

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

// geoBand is the hypsometric ramp over bedrock elevation: abyssal
// blues below the waves, tans and umbers over the lowlands, gray
// stone to white peaks. Biomes are ignored — this is the rock.
func geoBand(elev float64) (string, bool) {
	switch {
	case elev < -2000:
		return "17", false // abyssal
	case elev < -500:
		return "19", false // deep basin
	case elev < 0:
		return "26", false // shelf
	case elev < 200:
		return "65", false // coastal lowland
	case elev < 800:
		return "137", false // plain tan
	case elev < 1500:
		return "95", false // upland umber
	case elev < 2500:
		return "244", false // stone gray
	case elev < 3500:
		return "250", false // high stone
	default:
		return "255", true // peak white
	}
}

// BuildGeoGridBuf is the geological lens: pure elevation, no biome.
func BuildGeoGridBuf(cells []db.GetCellsInBoundsRow, rivers []db.GetRiverCellsInBoundsRow, roads []db.GetRoadCellsInBoundsRow, minX, minY, maxX, maxY int64) *GridBuf {
	return buildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY,
		func(kind string, elev float64, x, y int64) (string, bool) {
			return geoBand(elev)
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
	case "mountain", "cliff", "plateau", "pass", "unknown":
		return "245", false // barren rock
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
