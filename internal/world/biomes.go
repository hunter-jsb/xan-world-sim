package world

// Biome temperature gates — real ecological transitions:
//
//	freezePoint     — water freezes year-round; below this trees can't
//	                  sustain a closed canopy and we're in tundra
//	                  territory. Also gates marsh (frozen wetlands
//	                  aren't marshes) and lake liquidity.
//	warmCradleStart — closed temperate forest gives way to warmer
//	                  grassland/maquis above this. (Real-world MAT for
//	                  the temperate-warm transition; matches our
//	                  cradle's intended Mediterranean flavor.)
const (
	freezePoint     = 0.0
	warmCradleStart = 15.0
)

// refineBiomes splits bare cradle cells by temperature into forest
// (cool temperate) or tundra (cold but unfrozen). Foothills keep their
// topographic identity (the `n` glyph represents *hills*, not
// vegetation) so we don't biome-shift them.
func (w *World) refineBiomes() {
	for i := range w.Regions {
		rc := &w.Regions[i]
		if rc.RegionID != RegionCradle {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		t := Temperature(lat, rc.Elevation, w.Climate)
		switch {
		case t < freezePoint:
			rc.RegionID = RegionTundra
		case t < warmCradleStart:
			rc.RegionID = RegionForest
		}
	}
}

// marshRise is how far above the adjacent water's *surface* land can
// sit and still be wetland — the seasonal flood-stage / storm-surge
// scale (real-world river flood stages run 3–8m). Below it the water
// table reaches the roots; above it the bank is dry land no matter
// how close the water is.
const marshRise = 5.0

// markMarshes converts vegetated lowland within flood reach of a water
// body, where temperature is above freezing. Adjacency alone is not
// enough — a marsh forms where land sits within marshRise of the
// adjacent water *surface*: sea level for the seas (climate-driven),
// the spill surface for lakes (from bathymetry), and the channel's own
// elevation for rivers. An 80m coastal bluff next to the Brine stays
// dry; a river-mouth delta floods. The temperature gate is the same
// freezing-point used for lakes.
func (w *World) markMarshes(lakes []LakeCell) {
	seaLevel := w.Climate.SeaLevelDelta
	surfaceAt := make(map[[2]int]float64, len(lakes))
	for _, l := range lakes {
		surfaceAt[[2]int{int(l.X), int(l.Y)}] = l.Surface
	}

	waterLevel := make(map[[2]int]float64, len(w.Rivers))
	for _, rc := range w.Regions {
		p := [2]int{int(rc.X), int(rc.Y)}
		switch rc.RegionID {
		case RegionBrine, RegionEastSea:
			waterLevel[p] = seaLevel
		case RegionLake:
			if s, ok := surfaceAt[p]; ok {
				waterLevel[p] = s
			}
		}
	}
	g := gridOf(w.Regions)
	for _, r := range w.Rivers {
		p := [2]int{int(r.X), int(r.Y)}
		waterLevel[p] = g.elevAt(p)
	}

	for i := range w.Regions {
		rc := &w.Regions[i]
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionTundra:
		default:
			continue
		}
		flooded := false
		for _, d := range dirs8 {
			lvl, ok := waterLevel[[2]int{int(rc.X) + d[0], int(rc.Y) + d[1]}]
			if ok && rc.Elevation-lvl <= marshRise {
				flooded = true
				break
			}
		}
		if !flooded {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		if Temperature(lat, rc.Elevation, w.Climate) > freezePoint {
			rc.RegionID = RegionMarsh
		}
	}
}
