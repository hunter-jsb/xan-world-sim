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
	flipped := make(map[[2]int64]bool)
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
			flipped[[2]int64{rc.X, rc.Y}] = true
		}
	}
	if len(flipped) == 0 {
		return
	}
	// The lake carries the river. A river that reaches a basin pools
	// there until the basin overflows at its spill point, so inside
	// the lake there is no channel to draw — the trace continues to
	// the outlet and the downstream segment is the same river. Frozen
	// basins (not flipped) keep their river overlay: the ice hasn't
	// released the basin yet. Mirrors the seat filtering in placeSeats.
	filtered := w.Rivers[:0]
	for _, r := range w.Rivers {
		if flipped[[2]int64{r.X, r.Y}] {
			continue
		}
		filtered = append(filtered, r)
	}
	w.Rivers = filtered
}

// nameLakes runs last in the pipeline so any cells that became seats
// during transformations are excluded from the flood. Each water body
// gets one name, seeded from its lex-smallest cell, plus its
// bathymetry (water surface = the basin's spill level, max depth =
// deepest submerged point) from the LakeCell data the flow pass
// produced.
//
// Water bodies are flooded with the same surface-continuity rule as
// detection: adjacent RegionLake cells with different water surfaces
// are different (terraced) lakes and keep separate names. A lake
// fragmented by a settlement (rare; happens when a Tributary sits on
// a lake-cell river bend) also yields two names — geologically that's
// now two lakes.
func (w *World) nameLakes(lakes []LakeCell) {
	const surfTol = 0.5 // same tolerance as detection in flowRivers
	bathy := make(map[[2]int]LakeCell, len(lakes))
	for _, l := range lakes {
		bathy[[2]int{int(l.X), int(l.Y)}] = l
	}
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
	visited := make(map[[2]int]bool)
	var nextID int64 = 1
	for _, s := range keys {
		if visited[s] {
			continue
		}
		comp := [][2]int{s}
		visited[s] = true
		for i := 0; i < len(comp); i++ {
			head := comp[i]
			hs := bathy[head].Surface
			for _, d := range dirs8 {
				n := [2]int{head[0] + d[0], head[1] + d[1]}
				if !lakeAt[n] || visited[n] {
					continue
				}
				ns := bathy[n].Surface
				if ns-hs > surfTol || hs-ns > surfTol {
					continue // different water surface — separate lake
				}
				visited[n] = true
				comp = append(comp, n)
			}
		}
		rep := comp[0]
		var surface, depth float64
		for _, c := range comp {
			if c[1] < rep[1] || (c[1] == rep[1] && c[0] < rep[0]) {
				rep = c
			}
			if b, ok := bathy[c]; ok {
				if b.Surface > surface {
					surface = b.Surface
				}
				if b.Depth > depth {
					depth = b.Depth
				}
			}
		}
		w.Lakes = append(w.Lakes, LakeInfo{
			ID:          nextID,
			Name:        generateName(nameSeedForCell(w.Seed, int64(rep[0]), int64(rep[1]))),
			X:           int64(rep[0]),
			Y:           int64(rep[1]),
			SurfaceElev: surface,
			MaxDepth:    depth,
		})
		nextID++
	}
}
