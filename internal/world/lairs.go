package world

import "sort"

// peakCell is a local-maximum cell returned by findAndMarkPeaks.
type peakCell struct {
	x, y int
	elev float64
}

// findAndMarkPeaks finds local elevation maxima of targetZone cells in
// a (2*window+1)² window, greedy-deduplicates by Euclidean distance
// (minSepSq = min separation squared in cells), marks picked cells'
// RegionID as featureZone, and returns the picks sorted by elevation
// desc. E/S tiebreaker on equal-elevation neighbors: a cell wins ties
// against its E/S neighbors, loses to N/W — guarantees a unique winner
// per flat plateau.
func findAndMarkPeaks(regions []RegionCell, targetZone, featureZone int64, window, minSepSq int) []peakCell {
	g := gridOf(regions)
	var cands []peakCell
	for _, rc := range regions {
		if rc.RegionID != targetZone {
			continue
		}
		cx, cy := int(rc.X), int(rc.Y)
		d := g.elevAt([2]int{cx, cy})
		isMax := true
	checkNbrs:
		for dy := -window; dy <= window; dy++ {
			for dx := -window; dx <= window; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				n := [2]int{cx + dx, cy + dy}
				if g.regionAt(n) != targetZone {
					continue
				}
				nd := g.elevAt(n)
				if nd > d || (nd == d && (dy < 0 || (dy == 0 && dx < 0))) {
					isMax = false
					break checkNbrs
				}
			}
		}
		if isMax {
			cands = append(cands, peakCell{cx, cy, d})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].elev != cands[j].elev {
			return cands[i].elev > cands[j].elev
		}
		if cands[i].y != cands[j].y {
			return cands[i].y < cands[j].y
		}
		return cands[i].x < cands[j].x
	})
	var picks []peakCell
	for _, c := range cands {
		tooClose := false
		for _, p := range picks {
			ddx, ddy := c.x-p.x, c.y-p.y
			if ddx*ddx+ddy*ddy < minSepSq {
				tooClose = true
				break
			}
		}
		if !tooClose {
			picks = append(picks, c)
		}
	}
	peakSet := make(map[[2]int64]bool, len(picks))
	for _, p := range picks {
		peakSet[[2]int64{int64(p.x), int64(p.y)}] = true
	}
	for i := range regions {
		if peakSet[[2]int64{regions[i].X, regions[i].Y}] {
			regions[i].RegionID = featureZone
		}
	}
	return picks
}

// placeLairs marks the three dragon-family lair tiers, spaced by
// territory size:
//
//	Dragon dens      — mountain peaks at strict local elevation max in
//	                   a 5×5 window, min-sep 6 cells (~300km per
//	                   dragon territory).
//	Drake nests      — foothill peaks, min-sep 4 cells (~200km).
//	                   Drakes are "the everyday menace" — more numerous
//	                   and closer-spaced than dragons.
//	Wyvern rookeries — cliff peaks, min-sep 3 cells (~150km). Wyverns
//	                   "nest like raptors — often colonial," so they
//	                   crowd more tightly than drakes or dragons.
func (w *World) placeLairs() {
	for i, p := range findAndMarkPeaks(w.Regions, RegionMountain, RegionDragonDen, 2, 6*6) {
		w.Dens = append(w.Dens, DenInfo{
			ID:        int64(i + 1),
			Name:      generateName(nameSeedForCell(w.Seed, int64(p.x), int64(p.y))),
			X:         int64(p.x),
			Y:         int64(p.y),
			Elevation: p.elev,
		})
	}
	for i, p := range findAndMarkPeaks(w.Regions, RegionFoothill, RegionDrakeNest, 2, 4*4) {
		w.Nests = append(w.Nests, NestInfo{
			ID:        int64(i + 1),
			Name:      generateName(nameSeedForCell(w.Seed, int64(p.x), int64(p.y))),
			X:         int64(p.x),
			Y:         int64(p.y),
			Elevation: p.elev,
		})
	}
	for i, p := range findAndMarkPeaks(w.Regions, RegionCliff, RegionWyvernRookery, 1, 3*3) {
		w.Rookeries = append(w.Rookeries, RookeryInfo{
			ID:        int64(i + 1),
			Name:      generateName(nameSeedForCell(w.Seed, int64(p.x), int64(p.y))),
			X:         int64(p.x),
			Y:         int64(p.y),
			Elevation: p.elev,
		})
	}
}

// findPasses marks saddles in the ridge that bridge the cradle to the
// plateau. From the lore: "pre-Melt these were passable; the Melt made
// them spectacular and brutal." Detection signals:
//
//  1. The cell is itself a mountain (it sits *on* the ridge).
//  2. Its elevation is ≤ all mountain cells in a 5×5 window (locally
//     lowest along the ridge axis — the saddle). A 3×3 window
//     over-counts because every short rise+dip in the smoothed
//     elevation registers; 5×5 only flags cells that dominate a
//     meaningful stretch of ridge (~250km at our cell size).
//  3. It has at least one foothill/cradle/forest/tundra cell to its
//     south — meaning the cradle side is reachable from this point.
//     Without (3) the saddle dead-ends inside the mountain band and
//     isn't a real "pass through."
//
// E/S tiebreaker on equal-elevation neighbors so a flat ridge-top
// doesn't yield clusters of passes.
func (w *World) findPasses() {
	g := gridOf(w.Regions)
	isApproachKind := func(id int64) bool {
		return id == RegionFoothill || id == RegionCradle ||
			id == RegionForest || id == RegionTundra ||
			id == RegionMarsh
	}
	const passWindow = 2
	var picks [][2]int
	for i := range w.Regions {
		rc := &w.Regions[i]
		if rc.RegionID != RegionMountain {
			continue
		}
		cx, cy := int(rc.X), int(rc.Y)
		d := g.elevAt([2]int{cx, cy})
		isMin := true
		hasMtnNbr := false
		hasApproach := false
		for dy := -passWindow; dy <= passWindow && isMin; dy++ {
			for dx := -passWindow; dx <= passWindow && isMin; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				n := [2]int{cx + dx, cy + dy}
				nid, nok := g.region[n]
				if !nok {
					continue
				}
				if nid == RegionMountain {
					hasMtnNbr = true
					nd := g.elevAt(n)
					if nd < d {
						isMin = false
					} else if nd == d {
						// E/S tiebreaker: lose ties to N/W
						if dy < 0 || (dy == 0 && dx < 0) {
							isMin = false
						}
					}
				}
				// "South approach" remains the immediate row below (we
				// want a foothill/cradle directly accessible from the
				// saddle, not several cells away).
				if dy == 1 && (dx >= -1 && dx <= 1) && isApproachKind(nid) {
					hasApproach = true
				}
			}
		}
		if isMin && hasMtnNbr && hasApproach {
			picks = append(picks, [2]int{cx, cy})
		}
	}
	sortYX(picks)
	passSet := make(map[[2]int64]bool, len(picks))
	for _, p := range picks {
		passSet[[2]int64{int64(p[0]), int64(p[1])}] = true
	}
	for i := range w.Regions {
		rc := &w.Regions[i]
		if passSet[[2]int64{rc.X, rc.Y}] {
			rc.RegionID = RegionPass
		}
	}
	for i, p := range picks {
		w.Passes = append(w.Passes, PassInfo{
			ID:   int64(i + 1),
			Name: generateName(nameSeedForCell(w.Seed, int64(p[0]), int64(p[1]))),
			X:    int64(p[0]),
			Y:    int64(p[1]),
		})
	}
}
