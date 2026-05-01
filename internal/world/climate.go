package world

// World coordinates: a global lat/lon system, equator at 0°, north
// pole at +90°, south pole at -90°. The 60x22 map sits in the
// northern hemisphere at mid-to-high latitudes — roughly Anatolia
// (south edge) up to Scandinavia (north edge), give or take.
//
// Per-row latitude is linear interpolation between LatTop (y=0) and
// LatBottom (y=Height-1). This is the foundation everything else
// climate-related hangs off: temperature gradient, glacier extent,
// solar insolation, etc.
const (
	DefaultLatTop    = 55.0 // degrees North, top edge of map
	DefaultLatBottom = 30.0 // degrees North, bottom edge of map
)

// Latitude returns the degrees-N for map row y, given the latitude
// bounds of the map. y=0 is northern edge (LatTop), y=Height-1 is
// southern edge (LatBottom).
func Latitude(y int, latTop, latBottom float64) float64 {
	if Height <= 1 {
		return (latTop + latBottom) / 2
	}
	frac := float64(y) / float64(Height-1)
	return latTop - frac*(latTop-latBottom)
}

// OrbitalParams describes the planet's current orbital configuration —
// the Milankovitch knobs. These vary on slow cycles (~26kya, ~41kya,
// ~100kya) and drive long-term climate change. Stored on the World;
// not yet *consumed* by anything (next pass: derive ClimateState from
// these instead of hardcoding per era).
type OrbitalParams struct {
	// Obliquity: axial tilt in degrees. Earth: ~22.1° to ~24.5°
	// (cycle ~41kya). Higher obliquity = stronger seasons + more
	// summer melt at high latitude = ice sheets retreat.
	Obliquity float64

	// Eccentricity: how elliptical the orbit is. Earth: ~0.003 to
	// ~0.058 (cycle ~100kya). Modulates the strength of precession.
	Eccentricity float64

	// Precession: longitude of perihelion in degrees. Cycle ~26kya.
	// Determines which season the planet is closest to the sun.
	Precession float64
}

// ClimateState is the climate at a moment in time. Right now it's
// hand-filled per era; later it should be *derived* from
// OrbitalParams + a deep-time clock so the climate emerges from the
// orbital model rather than being declared.
type ClimateState struct {
	// SeaLevelDelta is meters relative to present sea level.
	// Negative during glacial peaks (water locked in ice sheets).
	SeaLevelDelta float64

	// GlacialIndex: 0 = present-day interglacial, 1 = full glacial peak.
	GlacialIndex float64

	// GlobalMeanTempDelta: degrees C relative to present global mean.
	GlobalMeanTempDelta float64
}

// EarthNow approximates present-day Earth orbital configuration.
func EarthNow() OrbitalParams {
	return OrbitalParams{
		Obliquity:    23.44,
		Eccentricity: 0.0167,
		Precession:   102.95,
	}
}

// EarthGlacialPeak approximates an orbital configuration favorable to
// continental ice sheet growth — low NH summer insolation. Numbers
// are illustrative, not the literal 21kya Earth values.
func EarthGlacialPeak() OrbitalParams {
	return OrbitalParams{
		Obliquity:    22.5,
		Eccentricity: 0.020,
		Precession:   180.0,
	}
}

// ClimateForEra returns the hand-tuned climate state for an era.
// This is the bridge between the era system and a future climate-
// driven worldgen pass.
func ClimateForEra(era Era) ClimateState {
	switch era {
	case EraOldWorld:
		return ClimateState{
			SeaLevelDelta:       -120,
			GlacialIndex:        1.0,
			GlobalMeanTempDelta: -8.0,
		}
	default:
		return ClimateState{
			SeaLevelDelta:       0,
			GlacialIndex:        0.0,
			GlobalMeanTempDelta: 0,
		}
	}
}

// OrbitalForEra returns approximate orbital params for an era.
func OrbitalForEra(era Era) OrbitalParams {
	switch era {
	case EraOldWorld:
		return EarthGlacialPeak()
	default:
		return EarthNow()
	}
}

// glacierThreshold is the annual-mean surface temperature below which a
// cell glaciates (in zones that *can* glaciate). Tuned so that the
// present-day cradle stays land at lat ~37°N and the glacial-peak
// cradle freezes; can be lifted/lowered to make worlds icier/warmer.
const glacierThreshold = -2.0

// canGlaciate decides which bedrock zones are *visually* allowed to
// turn into glacier when the temperature drops below threshold.
//
// Mountains, cliffs, the doab, and the high plateau stay rendered as
// themselves even when frozen — they're tall enough that we want the
// rocky identity to read through (real-world Alps are partly glaciated
// at high elevation but still read as "mountains," not "ice sheet").
//
// The deep Brine basin can't freeze in our model — too thermally
// inertial; a real saline body of that depth wouldn't freeze through.
func canGlaciate(zone BedrockZone) bool {
	switch zone {
	case BZMountain, BZCliff, BZPlateau, BZDoab, BZBrineDeep:
		return false
	default:
		return true
	}
}

// Temperature returns the approximate annual-mean surface temperature
// in degrees C at a cell with the given latitude (degrees N), bedrock
// elevation (m relative to present sea level), and current climate
// state.
//
// Rough model:
//   - base(lat) is a linear cosine-ish approximation: ~30°C at the
//     equator, falling 0.5°C per degree of latitude.
//   - Lapse rate is the standard ~6.5°C per kilometer of *positive*
//     elevation (below sea level we use surface elevation = 0).
//   - The climate's GlobalMeanTempDelta is amplified at high latitudes
//     (real-world polar amplification ~2x): factor 1 + |lat|/40.
//
// This is intentionally simple. It produces qualitatively correct
// behavior across our two eras without needing an atmospheric sim.
func Temperature(lat, elev float64, c ClimateState) float64 {
	base := 30.0 - 0.5*absFloat(lat)
	surfaceElev := elev
	if surfaceElev < 0 {
		surfaceElev = 0
	}
	lapse := -6.5 * surfaceElev / 1000.0
	delta := c.GlobalMeanTempDelta * latAmplification(lat)
	return base + lapse + delta
}

func latAmplification(lat float64) float64 {
	return 1.0 + absFloat(lat)/40.0
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
