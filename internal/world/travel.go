package world

// TravelCost returns the base movement cost to enter a cell of the
// given region kind (the string stored in the DB). Returns -1 if the
// terrain is impassable. River presence is handled by the caller.
//
// This is the canonical cost table for overland foot travel. Road
// construction uses a subset of these values (plateau and dragon lairs
// become impassable for road-building purposes).
func TravelCost(kind string) int {
	switch kind {
	case "seat", "march", "headwater", "outhold", "reach":
		return 2
	case "pass":
		return 3
	case "cradle", "forest", "tundra", "agraria", "agraria_upland":
		return 4
	case "foothill":
		return 5
	case "doab":
		return 6
	case "marsh":
		return 8
	case "plateau":
		return 15
	case "den", "nest", "rookery":
		return 25
	}
	return -1 // mountain, cliff, sea, glacier, lake, drowned, unknown
}
