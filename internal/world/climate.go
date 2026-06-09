package world

import "math"

// World coordinates: a global lat/lon system, equator at 0°, north
// pole at +90°, south pole at -90°. The 120×50 map sits in the
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

// Milankovitch cycle periods (kiloyears).
//
// oblPeriod=410 places the obliquity maximum at kya=0 (warm present) and
// the obliquity minimum at kya=205 (cold LGM), giving a single smooth
// warm→cold arc across the canonical sim range. GlacialIndex is driven
// by obliquity alone so the sea-level gradient is steady (~5m per 5-ka
// keypress). eccPeriod=97 and precPeriod=23 are close to Earth's real
// values; they shape the displayed OrbitalParams but do not affect
// GlacialIndex.
const (
	oblPeriod  = 410.0 // obliquity — half-period matches kya=0→205 warm→cold arc
	eccPeriod  = 97.0  // eccentricity (Earth ≈ 100 ka)
	precPeriod = 23.0  // precession  (Earth ≈ 26 ka effective)

	oblMean = 23.3 // degrees axial tilt, midpoint of oscillation
	oblAmp  = 1.2  // degrees amplitude — range 22.1° to 24.5°
	eccMean = 0.030
	eccAmp  = 0.020 // range 0.010 to 0.050
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

// OrbitalParams describes the planet's orbital configuration at a
// given kya — the three Milankovitch knobs. Computed independently
// from their respective cycles; used to derive NH summer insolation,
// which drives GlacialIndex and ClimateState.
type OrbitalParams struct {
	// Obliquity: axial tilt in degrees. Cycle ~38 ka (Earth: ~41 ka).
	// Higher obliquity = stronger seasons = more summer melt at high
	// latitude = ice sheets retreat.
	Obliquity float64

	// Eccentricity: orbital ellipticity. Cycle ~97 ka (Earth: ~100 ka).
	// Modulates how strongly precession affects seasonal insolation.
	Eccentricity float64

	// Precession: longitude of perihelion in degrees. Cycle ~23 ka
	// (Earth: ~26 ka effective). Determines which season the planet
	// is closest to the sun.
	Precession float64
}

// ClimateState is the climate at a moment in time, derived from
// Milankovitch orbital forcing rather than declared per era.
type ClimateState struct {
	// SeaLevelDelta is meters relative to present sea level.
	// Negative during glacial peaks (water locked in ice sheets).
	SeaLevelDelta float64

	// GlacialIndex: 0 = warmest interglacial in [0, KyaMax],
	// 1 = coldest glacial peak.
	GlacialIndex float64

	// GlobalMeanTempDelta: degrees C relative to present global mean.
	GlobalMeanTempDelta float64
}

// OrbitalAt returns the orbital configuration at a given kya, computed
// independently for each Milankovitch cycle.
//
// Phase conventions at kya=0 (present-day Holocene interglacial):
//   - Obliquity at maximum (24.5°) — next peak at kya=410
//   - Eccentricity at minimum (0.010) — next trough at kya=97
//   - Precession ω=90° — perihelion near NH summer solstice, slightly
//     favouring warm NH summers (Earth's actual ω is ~103°).
func OrbitalAt(kya int) OrbitalParams {
	t := float64(kya)
	return OrbitalParams{
		Obliquity:    oblMean + oblAmp*math.Cos(2*math.Pi*t/oblPeriod),
		Eccentricity: eccMean - eccAmp*math.Cos(2*math.Pi*t/eccPeriod),
		Precession:   math.Mod(90.0+360.0/precPeriod*t, 360.0),
	}
}

// GlacialIndex returns the climate-cycle position at a given kya,
// in [0, 1]: 0 = warmest interglacial (kya=0), 1 = coldest glacial peak (kya=205).
//
// Driven by obliquity alone so that the gradient is smooth and steady
// across the sim's navigation range (~5m sea-level change per 5-ka
// keypress). Eccentricity and precession appear in OrbitalAt for display
// but do not feed into ice-volume or temperature calculations.
func GlacialIndex(kya int) float64 {
	if kya < 0 {
		kya = 0
	}
	if kya > KyaMax {
		kya = KyaMax
	}
	obl := OrbitalAt(kya).Obliquity
	return (oblMean + oblAmp - obl) / (2 * oblAmp)
}

// ClimateAt returns the climate state at a given kya. Sea level and
// mean-temp delta scale linearly with the glacial index between the
// warm-peak (gI=0) and cold-peak (gI=1) anchor values.
func ClimateAt(kya int) ClimateState {
	gI := GlacialIndex(kya)
	return ClimateState{
		SeaLevelDelta:       -120 * gI,
		GlacialIndex:        gI,
		GlobalMeanTempDelta: -8 * gI,
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
