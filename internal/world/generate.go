package world

import (
	"math/rand"
	"sort"
)

// Generate produces a deterministic world from the given seed and a
// moment in geological time (kya = kiloyears before present).
//
// The pipeline is climate-driven and time-driven: a single bedrock
// model (zones + elevations) is built once from the seed and is
// stable across all kya — geology doesn't move on these timescales.
// Climate (sea level, mean temp delta) at the given kya is then
// applied per cell to derive whether the cell shows up as land,
// sea, or glacier. As kya scrubs from 205 toward 0, the ice retreats
// smoothly and Agraria submerges — both as consequences of the
// climate cycle, not hardcoded snapshots.
//
// Rivers remain hand-laid for now and exist only when GlacialIndex
// is low (post-Melt-ish climate). The glacial-peak world has no
// rivers because the meltwater hasn't been released yet.
func Generate(seed int64, kya int) World {
	rng := rand.New(rand.NewSource(seed))
	bedrock := generateBedrock(rng)

	climate := ClimateAt(kya)

	w := World{
		Seed:      seed,
		Kya:       kya,
		Era:       EraForKya(kya),
		LatTop:    DefaultLatTop,
		LatBottom: DefaultLatBottom,
		Orbital:   OrbitalAt(kya),
		Climate:   climate,
	}

	for y := 0; y < Height; y++ {
		lat := Latitude(y, w.LatTop, w.LatBottom)
		for x := 0; x < Width; x++ {
			b := bedrock[y][x]
			rid := classify(b, lat, climate)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID:  rid,
					X:         int64(x),
					Y:         int64(y),
					Elevation: b.Elevation,
				})
			}
		}
	}

	// Rivers grow head-to-mouth as climate warms. Threshold is uniform
	// (it just identifies the river network topology); the maximum
	// length each river extends from its headwater scales with
	// glacial index. At the glacial peak, length=0 (no rivers — water
	// locked in ice). As warming progresses, headwaters appear first,
	// then rivers extend downstream. By the time the cycle is fully
	// warm, rivers reach all the way from headwater to sea.
	//
	// Lakes are a side-product: cells where pit-fill identified a
	// basin floor (flow target is higher in bedrock terms). Convert
	// eligible cradle/foothill cells in-place so the renderer paints
	// them as lakes. Cells that are currently glaciated (e.g., at the
	// glacial peak when the cradle is under ice) keep their glacier
	// classification — lakes are buried until the ice retreats, which
	// is geologically correct.
	var lakes []LakeCell
	w.RiverInfo, w.Rivers, lakes = flowRivers(bedrock,
		riverThreshold,
		riverMaxLenFor(climate.GlacialIndex))

	if len(lakes) > 0 {
		lakeSet := make(map[[2]int]bool, len(lakes))
		for _, l := range lakes {
			lakeSet[[2]int{int(l.X), int(l.Y)}] = true
		}
		// A geological basin renders as a *lake* only when the cell's
		// temperature is above freezing. Below freezing, the basin
		// holds ice — and our classifier already routes cold land to
		// RegionGlacier, so non-glaciated cradle/foothill in basins
		// is exactly the "liquid" case. Temperature is recomputed per
		// cell because lapse rate makes higher cells colder than
		// lower cells at the same latitude.
		for i := range w.Regions {
			rc := &w.Regions[i]
			if !lakeSet[[2]int{int(rc.X), int(rc.Y)}] {
				continue
			}
			if rc.RegionID != RegionCradle && rc.RegionID != RegionFoothill {
				continue
			}
			lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
			if Temperature(lat, rc.Elevation, climate) > 0 {
				rc.RegionID = RegionLake
			}
		}
	}

	// Biome refinement: split bare cradle cells by temperature into
	// forest (cool temperate) or tundra (cold but unfrozen). The two
	// gates are real ecological transitions:
	//   0°C  — water freezes year-round; below this trees can't sustain
	//          a closed canopy and we're in tundra territory.
	//   15°C — closed temperate forest gives way to warmer
	//          grassland/maquis above this. (Real-world MAT for the
	//          temperate-warm transition; matches our cradle's
	//          intended Mediterranean flavor at warmer values.)
	// Foothills keep their topographic identity (the `n` glyph
	// represents *hills*, not vegetation) so we don't biome-shift them.
	const (
		freezePoint     = 0.0
		warmCradleStart = 15.0
	)
	for i := range w.Regions {
		rc := &w.Regions[i]
		if rc.RegionID != RegionCradle {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		t := Temperature(lat, rc.Elevation, climate)
		switch {
		case t < freezePoint:
			rc.RegionID = RegionTundra
		case t < warmCradleStart:
			rc.RegionID = RegionForest
		}
	}

	// Lord seats — three tiers, each with a distinct geographic
	// signature drawn from the lore typology in `region.md`:
	//
	//   Tributary — midpoint of a river of length ≥ 5. The salmon-lord
	//               hall on a navigable stretch. Scale-gated: shorter
	//               rivers can't sustain a lord at our cell size.
	//   Headwater — head (Ord=1) of a river of length ≥ 10. Sacred
	//               sources at continental rivers; twice the Tributary
	//               scale because Headwater holds are bigger seats
	//               (closest to dwarves, contested by religious orders).
	//   March    — foothill/cradle directly adjacent to a connected
	//              mountain massif of ≥3 cells. Geographically the
	//              "wall" — defense against the mountain wilds is the
	//              seat's reason to exist. One per massif, at the
	//              highest perimeter cell (most defensible).
	//
	// All three are climate-coupled through the layers below them
	// (rivers vanish at LGM → no Tributary or Headwater; mountains
	// stay → Marches persist through ice ages, which matches the lore:
	// March lineages are the *oldest*, since the mountain is forever).
	seatSet := make(map[[2]int64]int64)
	if len(w.Rivers) > 0 {
		groups := make(map[int64][]RiverCell)
		for _, r := range w.Rivers {
			groups[r.RiverID] = append(groups[r.RiverID], r)
		}
		for _, group := range groups {
			sort.Slice(group, func(i, j int) bool { return group[i].Ord < group[j].Ord })
			if len(group) >= 5 {
				mid := group[len(group)/2]
				seatSet[[2]int64{mid.X, mid.Y}] = RegionSeat
			}
			if len(group) >= 10 {
				head := group[0]
				if _, taken := seatSet[[2]int64{head.X, head.Y}]; !taken {
					seatSet[[2]int64{head.X, head.Y}] = RegionHeadwater
				}
			}
		}
	}
	// March detection: BFS-flood mountain cells into massifs, then for
	// each massif of meaningful size pick the highest-elevation
	// foothill/cradle cell touching it. Don't overwrite existing seats
	// (Tributary or Headwater) — if the mountain massif's natural
	// March cell is already a river-tier seat, the role doubles up
	// and we just keep the river-tier label. This is fine: the
	// typology in lore explicitly bleeds (a Tributary on a wall-
	// adjacent stretch *is* a March in spirit).
	{
		const minMassifCells = 3
		regionAt := make(map[[2]int]int64, len(w.Regions))
		elevAt := make(map[[2]int]float64, len(w.Regions))
		for _, rc := range w.Regions {
			regionAt[[2]int{int(rc.X), int(rc.Y)}] = rc.RegionID
			elevAt[[2]int{int(rc.X), int(rc.Y)}] = rc.Elevation
		}
		visited := make(map[[2]int]bool)
		isMountain := func(p [2]int) bool { return regionAt[p] == RegionMountain }
		isWallish := func(p [2]int) bool {
			id := regionAt[p]
			return id == RegionFoothill || id == RegionCradle ||
				id == RegionForest || id == RegionTundra ||
				id == RegionMarsh
		}
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				p := [2]int{x, y}
				if !isMountain(p) || visited[p] {
					continue
				}
				var massif [][2]int
				queue := [][2]int{p}
				visited[p] = true
				for len(queue) > 0 {
					head := queue[0]
					queue = queue[1:]
					massif = append(massif, head)
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if dx == 0 && dy == 0 {
								continue
							}
							n := [2]int{head[0] + dx, head[1] + dy}
							if isMountain(n) && !visited[n] {
								visited[n] = true
								queue = append(queue, n)
							}
						}
					}
				}
				if len(massif) < minMassifCells {
					continue
				}
				best := [2]int{-1, -1}
				bestElev := -1e9
				for _, m := range massif {
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							if dx == 0 && dy == 0 {
								continue
							}
							n := [2]int{m[0] + dx, m[1] + dy}
							if !isWallish(n) {
								continue
							}
							if elevAt[n] > bestElev {
								bestElev = elevAt[n]
								best = n
							}
						}
					}
				}
				if best[0] < 0 {
					continue
				}
				key := [2]int64{int64(best[0]), int64(best[1])}
				if _, taken := seatSet[key]; !taken {
					seatSet[key] = RegionMarch
				}
			}
		}
	}
	if len(seatSet) > 0 {
		for i := range w.Regions {
			rc := &w.Regions[i]
			if id, ok := seatSet[[2]int64{rc.X, rc.Y}]; ok {
				rc.RegionID = id
			}
		}
		// Filter river cells that landed on a seat — the directional
		// river glyph would paint over the seat marker. River presence
		// is implicit (the seat sits on it).
		filtered := w.Rivers[:0]
		for _, r := range w.Rivers {
			if _, ok := seatSet[[2]int64{r.X, r.Y}]; ok {
				continue
			}
			filtered = append(filtered, r)
		}
		w.Rivers = filtered
	}

	// Marsh: vegetated lowland directly adjacent to a water body, where
	// temperature is above freezing. The "adjacency to water" criterion
	// is the wet-biome definition; the temperature gate is the same
	// freezing-point used for lakes — frozen wetlands aren't marshes.
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
		// Check 8-neighbors for water adjacency.
		adjacent := false
		for dy := -1; dy <= 1 && !adjacent; dy++ {
			for dx := -1; dx <= 1 && !adjacent; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				if waterSet[[2]int{int(rc.X) + dx, int(rc.Y) + dy}] {
					adjacent = true
				}
			}
		}
		if !adjacent {
			continue
		}
		lat := Latitude(int(rc.Y), w.LatTop, w.LatBottom)
		if Temperature(lat, rc.Elevation, climate) > freezePoint {
			rc.RegionID = RegionMarsh
		}
	}

	return w
}

// classify is the climate→surface mapper. Order of precedence:
//  1. Agraria shelf gets a "is exposed?" check first — when its
//     elevation is at or above sea level, it always reads as Agraria,
//     regardless of temperature. (Lore: temperate microclimate; the
//     Coastals lived there during glacial peaks, so it can't be ice.)
//  2. Glaciation, where the zone allows it. Glacier outranks
//     submerged-water — a frozen sea surface reads as glacier (ice
//     shelf), not sea.
//  3. Submerged water, mapped to whichever sea/basin the zone is in.
//  4. Otherwise the zone's exposed-land identity.
func classify(b BedrockCell, lat float64, climate ClimateState) int64 {
	seaLevel := climate.SeaLevelDelta

	// Shelf cells: when exposed they always read as Agraria (their
	// lore identity) regardless of temperature; when submerged they
	// stay as Brine (no "sea ice" intermediate — keeps the
	// emerge/submerge transition visually clean).
	if b.Zone == BZAgrariaShelf {
		if b.Elevation >= seaLevel {
			return RegionAgraria
		}
		return RegionBrine
	}
	if b.Zone == BZAgrariaUpland {
		if b.Elevation >= seaLevel {
			return RegionAgrariaUpland
		}
		return RegionBrine
	}

	if canGlaciate(b.Zone) {
		if Temperature(lat, b.Elevation, climate) < glacierThreshold {
			return RegionGlacier
		}
	}

	// Note: cliff zone classification happens in bedrockZone now (it's a
	// bedrock property, not a climate one). Code retained here in case
	// future climate effects need to know cliff vs mountain.
	if b.Elevation < seaLevel {
		switch b.Zone {
		case BZBrineDeep, BZAgrariaShelf, BZAgrariaUpland:
			return RegionBrine
		case BZEastBasin:
			return RegionEastSea
		default:
			// land zones aren't normally below sea level
			return RegionEastSea
		}
	}

	switch b.Zone {
	case BZPlateau:
		return RegionPlateau
	case BZMountain:
		return RegionMountain
	case BZCliff:
		return RegionCliff
	case BZFoothill:
		return RegionFoothill
	case BZDoab:
		return RegionDoab
	case BZCradle:
		return RegionCradle
	case BZAgrariaShelf:
		return RegionAgraria
	case BZAgrariaUpland:
		return RegionAgrariaUpland
	case BZEastBasin:
		// Exposed (e.g., extreme low-stand) — reads as cradle-ish land.
		return RegionCradle
	case BZBrineDeep:
		// Should not normally happen; deep basin shouldn't be exposed.
		return RegionUnknown
	}
	return 0
}

// ----- bedrock-procgen helpers (used by generateBedrock) -----

func genMountainRow(rng *rand.Rand) []int {
	out := make([]int, Width)
	jitter := 0
	for x := Width - 1; x >= 0; x-- {
		base := baseMountainRow(x)
		if base < 0 {
			out[x] = -1
			continue
		}
		jitter += rng.Intn(3) - 1
		jitter = clamp(jitter, -2, 2)
		mr := base + jitter
		mr = clamp(mr, 1, Height-3)
		out[x] = mr
	}
	return out
}

func genFoothillThickness(rng *rand.Rand) []int {
	out := make([]int, Width)
	for x := 0; x < Width; x++ {
		base := baseFoothillThickness(x)
		if base == 0 {
			out[x] = 0
			continue
		}
		ft := base + rng.Intn(3) - 1
		out[x] = clamp(ft, 0, 5)
	}
	return out
}

func genCoastX(rng *rand.Rand) []int {
	out := make([]int, Height)
	jitter := 0
	for y := 0; y < Height; y++ {
		jitter += rng.Intn(3) - 1
		jitter = clamp(jitter, -2, 2)
		out[y] = clamp(52+jitter, 50, 56)
	}
	return out
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

