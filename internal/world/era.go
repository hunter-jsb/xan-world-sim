package world

import "fmt"

// Era is a *display label* for a moment in geological time. The
// canonical state is now Kya (kiloyears before present); Era is a
// human-friendly tag derived from Kya (so the TUI title can say "now"
// or "near LGM" rather than "0kya" / "205kya" when those align with
// named historical moments).
type Era string

const (
	EraNow      Era = "now"
	EraOldWorld Era = "205kya" // around the Last Glacial Maximum

	// Canonical kya values for the named eras.
	KyaNow      = 0
	KyaOldWorld = 205

	// KyaMax caps how far back the sim will run. Our lore covers ~250kya
	// of human history (the Dawn at ~250kya); 300 gives a little
	// pre-Dawn deep-time headroom. With oblPeriod=410 the model is a
	// single warm→cold arc: kya=0 is the warm present, kya=205 is the
	// coldest (LGM), and the pre-Dawn period (kya≈250-300) is still
	// quite cold (gI≈0.67-0.91) — humans emerged during a world largely
	// locked in ice.
	KyaMax = 300
)

// ParseEra accepts the named-era strings ("now", "205kya") for
// backward-compatible CLI flags. New code should prefer Kya directly.
func ParseEra(s string) (Era, error) {
	switch Era(s) {
	case EraNow, EraOldWorld:
		return Era(s), nil
	default:
		return "", fmt.Errorf("unknown era %q (expected %q or %q)", s, EraNow, EraOldWorld)
	}
}

func (e Era) Other() Era {
	if e == EraNow {
		return EraOldWorld
	}
	return EraNow
}

// Kya returns the canonical kiloyears-before-present for a named era.
func (e Era) Kya() int {
	switch e {
	case EraOldWorld:
		return KyaOldWorld
	default:
		return KyaNow
	}
}

// EraForKya returns a display label for a given kya. Adjacent values
// share a label (e.g., kya in [195, 215] is "near LGM"); values that
// don't match any named band fall back to a generic "{n}kya" string.
func EraForKya(kya int) Era {
	switch {
	case kya <= 15:
		return EraNow
	case kya >= 195 && kya <= 215:
		return EraOldWorld
	default:
		return Era(fmt.Sprintf("%dkya", kya))
	}
}
