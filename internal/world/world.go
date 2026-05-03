// Package world owns deterministic world generation.
// Generate() is a pure function of the seed and the layout constants below;
// the same seed always produces the same world.
package world

const (
	Width  = 80
	Height = 30

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
	RegionGlacier        int64 = 11
	RegionAgraria        int64 = 12
	RegionAgrariaUpland  int64 = 13
	RegionLake           int64 = 14
	RegionForest         int64 = 15
	RegionTundra         int64 = 16
	RegionMarsh          int64 = 17
	RegionSeat           int64 = 18
	RegionMarch          int64 = 19
	RegionHeadwater      int64 = 20
	RegionOuthold        int64 = 21
	RegionReach          int64 = 22
	RegionPass           int64 = 23
)

type RegionCell struct {
	RegionID  int64
	X, Y      int64
	Elevation float64 // bedrock elevation (m), persisted for renderer shading
}

type RiverCell struct {
	RiverID int64
	X, Y    int64
	Ord     int64
}

// NamedSeat is a settlement cell with a generated name. Each Tributary,
// March, and Headwater hold gets one. The Tier carries which kind of
// seat it is (RegionSeat / RegionMarch / RegionHeadwater) so render
// and persistence layers can distinguish them.
type NamedSeat struct {
	X, Y int64
	Tier int64
	Name string
}

// LakeInfo names a lake — one entry per connected cluster of RegionLake
// cells. X, Y is a representative cell (lex-smallest in the cluster) so
// downstream consumers can highlight the lake on a map.
type LakeInfo struct {
	ID   int64
	Name string
	X, Y int64
}

// PassInfo names a mountain pass — a saddle in the ridge that bridges
// the cradle to the plateau. One entry per RegionPass cell; the
// detection guarantees passes don't cluster.
type PassInfo struct {
	ID   int64
	Name string
	X, Y int64
}

type World struct {
	Seed int64

	// Kya is the canonical state — kiloyears before present. Climate,
	// orbital params, and the derived Era label all hang off this.
	Kya int

	// Era is a display-only label derived from Kya (e.g., "now" or
	// "near LGM"). Persisted alongside Kya for human-readable inspection.
	Era Era

	// Latitude bounds of the map (degrees N). Northern hemisphere.
	LatTop, LatBottom float64

	Orbital OrbitalParams
	Climate ClimateState

	Regions    []RegionCell
	RiverInfo  []River     // (id, name) — populated alongside Rivers
	Rivers     []RiverCell // (river_id, x, y, ord)
	Seats      []NamedSeat // settlements (Tributary, March, Headwater, Reach, Outhold)
	Lakes      []LakeInfo  // named lake clusters (one per connected component)
	Passes     []PassInfo  // mountain passes through the ridge
}
