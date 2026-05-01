package world

import "math/rand"

// generateOldWorld is the world at the last glacial peak (~205kya):
//
//   - Sea level much lower; the Brine has receded toward the SW basin
//     and **Agraria** is exposed as a low crescent in the NW.
//   - The Eastern Sea basin (everything east of the Mountain Barrier)
//     is occupied by a continental ice sheet.
//   - Most of the future Cradle, south of the Mountain Barrier, is
//     also under the ice sheet.
//   - The Plateau and Mountain Barrier still exist (geology is stable
//     over 200kya); the mountains are not yet sharpened by the Melt.
//   - No rivers — those are Melt-era features that only form once the
//     ice sheet retreats.
//
// The bedrock shape (mountain row jitter) is shared with generateNow
// so a given seed produces matched-bedrock worlds across eras. The
// foothill jitter is reused but visually consumed by the ice sheet.
// The east-coast jitter is skipped because there is no east coast yet.
func generateOldWorld(seed int64) World {
	rng := rand.New(rand.NewSource(seed))

	mountainRow := genMountainRow(rng)
	_ = genFoothillThickness(rng) // consume RNG so seeds align across eras
	_ = genCoastX(rng)

	w := World{
		Seed: seed, Era: EraOldWorld,
		LatTop: DefaultLatTop, LatBottom: DefaultLatBottom,
		Orbital: OrbitalForEra(EraOldWorld),
		Climate: ClimateForEra(EraOldWorld),
	}

	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			rid := classifyOldWorld(x, y, mountainRow)
			if rid > 0 {
				w.Regions = append(w.Regions, RegionCell{
					RegionID: rid, X: int64(x), Y: int64(y),
				})
			}
		}
	}
	// no rivers in this era
	return w
}

func classifyOldWorld(x, y int, mountainRow []int) int64 {
	// West strip: Agraria exposed in the NW, deep Brine in the SW.
	if x <= 1 {
		if y <= 7 {
			return RegionAgraria
		}
		return RegionBrine
	}
	// East strip: continental ice sheet (the future Eastern Sea basin).
	if x >= 52 {
		return RegionGlacier
	}
	mr := mountainRow[x]
	if y < mr {
		return RegionPlateau
	}
	if y == mr {
		if x <= 11 {
			return RegionCliff
		}
		return RegionMountain
	}
	// South of the Mountain Barrier: the cradle is under the ice sheet.
	return RegionGlacier
}
