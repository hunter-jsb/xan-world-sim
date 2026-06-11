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

// markMarshes converts vegetated lowland directly adjacent to a water
// body, where temperature is above freezing. The "adjacency to water"
// criterion is the wet-biome definition; the temperature gate is the
// same freezing-point used for lakes.
func (w *World) markMarshes() {
	waterSet := make(map[[2]int]bool, len(w.Regions)+len(w.Rivers))
	for _, rc := range w.Regions {
		switch rc.RegionID {
		case RegionLake, RegionBrine, RegionEastSea:
			waterSet[[2]int{int(rc.X), int(rc.Y)}] = true
		}
	}
	for _, r := range w.Rivers {
		waterSet[[2]int{int(r.X), int(r.Y)}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		switch rc.RegionID {
		case RegionCradle, RegionForest, RegionTundra:
		default:
			continue
		}
		adjacent := false
		for _, d := range dirs8 {
			if waterSet[[2]int{int(rc.X) + d[0], int(rc.Y) + d[1]}] {
				adjacent = true
				break
			}
		}
		if !adjacent {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		if Temperature(lat, rc.Elevation, w.Climate) > freezePoint {
			rc.RegionID = RegionMarsh
		}
	}
}
