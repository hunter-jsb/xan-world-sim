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
