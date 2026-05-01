package world

import "fmt"

// Era is a snapshot in geological/climatic time. The bedrock geography
// (mountain barrier shape, plateau extent) is stable across eras; what
// changes is sea level, glacier extent, river presence, and exposed
// vs drowned shelves.
type Era string

const (
	// EraNow — present day, post-Melt (~10kya onward). Eastern Sea full,
	// Brine at present level, Agraria drowned, rivers flowing.
	EraNow Era = "now"

	// EraOldWorld — last glacial peak (~205kya, mid-Pleistocene). Sea
	// level much lower. Agraria exposed in the NW. Eastern Sea basin
	// occupied by a continental ice sheet. Cradle under glacier. No
	// rivers (those are Melt-era features).
	EraOldWorld Era = "205kya"
)

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
