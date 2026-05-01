// Package world owns deterministic world generation.
// Generate() is a pure function of the seed and the layout constants below;
// the same seed always produces the same world.
package world

const (
	Width  = 60
	Height = 22

	// Region IDs match the ones inserted by the migrations.
	RegionPlateau    int64 = 1
	RegionMountain   int64 = 2
	RegionCradle     int64 = 3
	RegionBrine      int64 = 4
	RegionEastSea    int64 = 5
	RegionUnknown    int64 = 6
	RegionDrowned    int64 = 7
	RegionDoab       int64 = 8
	RegionCliff      int64 = 9
	RegionFoothill   int64 = 10
	RegionGlacier    int64 = 11
	RegionAgraria    int64 = 12
)

type RegionCell struct {
	RegionID int64
	X, Y     int64
}

type RiverCell struct {
	RiverID int64
	X, Y    int64
	Ord     int64
}

type World struct {
	Seed int64
	Era  Era

	// Latitude bounds of the map (degrees N). Northern hemisphere.
	LatTop, LatBottom float64

	// Orbital + climate context — currently driven by era; later
	// will drive era (climate-derived worldgen).
	Orbital OrbitalParams
	Climate ClimateState

	Regions []RegionCell
	Rivers  []RiverCell
}
