package world

import (
	"math"
	"testing"
)

func TestGlacialIndex_Anchors(t *testing.T) {
	if gI := GlacialIndex(KyaNow); math.Abs(gI) > 1e-9 {
		t.Errorf("GlacialIndex(%d) = %g, want 0 (warm present)", KyaNow, gI)
	}
	if gI := GlacialIndex(KyaOldWorld); math.Abs(gI-1) > 1e-9 {
		t.Errorf("GlacialIndex(%d) = %g, want 1 (LGM)", KyaOldWorld, gI)
	}
}

func TestGlacialIndex_Clamps(t *testing.T) {
	if got, want := GlacialIndex(-50), GlacialIndex(0); got != want {
		t.Errorf("GlacialIndex(-50) = %g, want clamp to GlacialIndex(0) = %g", got, want)
	}
	if got, want := GlacialIndex(KyaMax+100), GlacialIndex(KyaMax); got != want {
		t.Errorf("GlacialIndex(KyaMax+100) = %g, want clamp to GlacialIndex(KyaMax) = %g", got, want)
	}
}

// TestGlacialIndex_SmoothMonotonicArc is the regression test for the
// "panning too fast" bug: across the navigable range kya=0..205 the
// index must rise monotonically with no per-step jumps. Max step over
// 1 ka is bounded by the obliquity derivative (~π/410 per ka ≈ 0.0077).
func TestGlacialIndex_SmoothMonotonicArc(t *testing.T) {
	const maxStep = 0.01
	prev := GlacialIndex(0)
	for k := 1; k <= KyaOldWorld; k++ {
		cur := GlacialIndex(k)
		if cur < prev {
			t.Fatalf("GlacialIndex not monotonic: gI(%d)=%g < gI(%d)=%g", k, cur, k-1, prev)
		}
		if cur-prev > maxStep {
			t.Fatalf("GlacialIndex jump at kya=%d: %g per 1 ka (max %g)", k, cur-prev, maxStep)
		}
		prev = cur
	}
	for k := 0; k <= KyaMax; k++ {
		gI := GlacialIndex(k)
		if gI < 0 || gI > 1 {
			t.Fatalf("GlacialIndex(%d) = %g out of [0,1]", k, gI)
		}
	}
}

func TestClimateAt_ScalesWithGlacialIndex(t *testing.T) {
	for _, kya := range []int{0, 50, 100, 205, KyaMax} {
		c := ClimateAt(kya)
		gI := GlacialIndex(kya)
		if c.GlacialIndex != gI {
			t.Errorf("ClimateAt(%d).GlacialIndex = %g, want %g", kya, c.GlacialIndex, gI)
		}
		if want := -120 * gI; c.SeaLevelDelta != want {
			t.Errorf("ClimateAt(%d).SeaLevelDelta = %g, want %g", kya, c.SeaLevelDelta, want)
		}
		if want := -8 * gI; c.GlobalMeanTempDelta != want {
			t.Errorf("ClimateAt(%d).GlobalMeanTempDelta = %g, want %g", kya, c.GlobalMeanTempDelta, want)
		}
	}
}

func TestOrbitalAt_PhaseConventions(t *testing.T) {
	o := OrbitalAt(0)
	if math.Abs(o.Obliquity-(oblMean+oblAmp)) > 1e-9 {
		t.Errorf("Obliquity at kya=0 = %g, want max %g", o.Obliquity, oblMean+oblAmp)
	}
	if math.Abs(o.Eccentricity-(eccMean-eccAmp)) > 1e-9 {
		t.Errorf("Eccentricity at kya=0 = %g, want min %g", o.Eccentricity, eccMean-eccAmp)
	}
	if math.Abs(o.Precession-90) > 1e-9 {
		t.Errorf("Precession at kya=0 = %g, want 90", o.Precession)
	}
	// One full precession cycle later, omega wraps back to 90.
	o23 := OrbitalAt(23)
	if math.Abs(o23.Precession-90) > 1e-9 {
		t.Errorf("Precession at kya=23 = %g, want 90 (full cycle)", o23.Precession)
	}
}

func TestLatitude_Interpolation(t *testing.T) {
	if got := Latitude(0, 55, 30); got != 55 {
		t.Errorf("Latitude(y=0) = %g, want 55 (top edge)", got)
	}
	if got := Latitude(Height-1, 55, 30); got != 30 {
		t.Errorf("Latitude(y=%d) = %g, want 30 (bottom edge)", Height-1, got)
	}
	mid := Latitude((Height-1)/2, 55, 30)
	if mid <= 30 || mid >= 55 {
		t.Errorf("Latitude(mid) = %g, want strictly between 30 and 55", mid)
	}
}

func TestTemperature_Gradients(t *testing.T) {
	c := ClimateAt(0)
	// Higher elevation is colder (lapse rate).
	if low, high := Temperature(40, 0, c), Temperature(40, 2000, c); high >= low {
		t.Errorf("lapse rate: T(2000m)=%g >= T(0m)=%g", high, low)
	}
	// Higher latitude is colder.
	if south, north := Temperature(30, 100, c), Temperature(55, 100, c); north >= south {
		t.Errorf("latitude gradient: T(55N)=%g >= T(30N)=%g", north, south)
	}
	// Below sea level clamps to surface elevation 0 — no negative lapse.
	if sea, basin := Temperature(40, 0, c), Temperature(40, -500, c); sea != basin {
		t.Errorf("sub-sea-level clamp: T(-500m)=%g != T(0m)=%g", basin, sea)
	}
	// Polar amplification: a glacial climate cools high latitudes more.
	lgm := ClimateAt(KyaOldWorld)
	dropSouth := Temperature(30, 100, c) - Temperature(30, 100, lgm)
	dropNorth := Temperature(55, 100, c) - Temperature(55, 100, lgm)
	if dropNorth <= dropSouth {
		t.Errorf("polar amplification: north drop %g <= south drop %g", dropNorth, dropSouth)
	}
}
