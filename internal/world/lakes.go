package world

// applyLakes converts eligible cells to RegionLake where pit-fill
// identified a basin floor (flow target is higher in bedrock terms).
//
// A geological basin renders as a *lake* only when the cell's
// temperature is above freezing. Below freezing, the basin holds ice —
// and our classifier already routes cold land to RegionGlacier, so
// non-glaciated cradle/foothill in basins is exactly the "liquid"
// case. Cells that are currently glaciated (e.g., at the glacial peak
// when the cradle is under ice) keep their glacier classification —
// lakes are buried until the ice retreats, which is geologically
// correct. Temperature is recomputed per cell because lapse rate makes
// higher cells colder than lower cells at the same latitude.
func (w *World) applyLakes(lakes []LakeCell) {
	if len(lakes) == 0 {
		return
	}
	lakeSet := make(map[[2]int]bool, len(lakes))
	for _, l := range lakes {
		lakeSet[[2]int{int(l.X), int(l.Y)}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		if !lakeSet[[2]int{int(rc.X), int(rc.Y)}] {
			continue
		}
		if rc.RegionID != RegionCradle && rc.RegionID != RegionFoothill {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		if Temperature(lat, rc.Elevation, w.Climate) > 0 {
			rc.RegionID = RegionLake
		}
	}
}

// nameLakes runs last in the pipeline so any cells that became seats
// during transformations are excluded from the flood. Each connected
// cluster of RegionLake cells gets one name, seeded from the cluster's
// lex-smallest cell. A lake fragmented by a settlement (rare; happens
// when a Tributary sits on a lake-cell river bend) yields two names —
// geologically that's now two lakes.
func (w *World) nameLakes() {
	lakeAt := make(map[[2]int]bool)
	for _, rc := range w.Regions {
		if rc.RegionID == RegionLake {
			lakeAt[[2]int{int(rc.X), int(rc.Y)}] = true
		}
	}
	var keys [][2]int
	for k := range lakeAt {
		keys = append(keys, k)
	}
	sortYX(keys)
	var nextID int64 = 1
	for _, comp := range components(keys, func(p [2]int) bool { return lakeAt[p] }) {
		rep := comp[0]
		for _, c := range comp {
			if c[1] < rep[1] || (c[1] == rep[1] && c[0] < rep[0]) {
				rep = c
			}
		}
		w.Lakes = append(w.Lakes, LakeInfo{
			ID:   nextID,
			Name: generateName(nameSeedForCell(w.Seed, int64(rep[0]), int64(rep[1]))),
			X:    int64(rep[0]),
			Y:    int64(rep[1]),
		})
		nextID++
	}
}
