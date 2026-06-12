// Package world owns deterministic world generation.
// Generate() is a pure function of the seed and the layout constants below;
// the same seed always produces the same world.
package world

const (
	Width  = 120
	Height = 50

	// Region IDs match the ones inserted by the migrations.
	RegionPlateau       int64 = 1
	RegionMountain      int64 = 2
	RegionCradle        int64 = 3
	RegionBrine         int64 = 4
	RegionEastSea       int64 = 5
	RegionUnknown       int64 = 6
	RegionDrowned       int64 = 7
	RegionDoab          int64 = 8
	RegionCliff         int64 = 9
	RegionFoothill      int64 = 10
	RegionGlacier       int64 = 11
	RegionAgraria       int64 = 12
	RegionAgrariaUpland int64 = 13
	RegionLake          int64 = 14
	RegionForest        int64 = 15
	RegionTundra        int64 = 16
	RegionMarsh         int64 = 17
	RegionSeat          int64 = 18
	RegionMarch         int64 = 19
	RegionHeadwater     int64 = 20
	RegionOuthold       int64 = 21
	RegionReach         int64 = 22
	RegionPass          int64 = 23
	RegionDragonDen     int64 = 24
	RegionDrakeNest     int64 = 25
	RegionWyvernRookery int64 = 26
	RegionCapital       int64 = 27
	RegionRuin          int64 = 28
	RegionVolcano       int64 = 29
	RegionLava          int64 = 30
)

type RegionCell struct {
	RegionID  int64
	X, Y      int64
	Elevation float64 // bedrock elevation (m), persisted for renderer shading
	Drainage  int64   // flow accumulation (upstream land cells), from the evolved bedrock
	Rock      int64   // topmost lithology (Rock* constants) — the geological lens reads it
	RockAge   int64   // ka before the world's moment the surface was laid
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
//
// Pressure is the seat's exposure to dragon raids — falls off with
// Chebyshev distance to the nearest dragon den, capped at 12 cells
// (~600km, the implied raid radius from lore). 0 = safe heartland;
// >8 = "constant dragon pressure" northern frontier.
//
// RealmID and Allegiance are set by the polity stages: which realm the
// seat belongs to, and how firmly the crown holds it (1 = the capital
// itself, 0 = beyond the crown's reach entirely).
type NamedSeat struct {
	X, Y       int64
	Tier       int64
	Name       string
	Pressure   float64
	RealmID    int64
	Allegiance float64
}

// Realm is one polity on the map: the Crown (the downstream heartland
// power centered on the capital) or an independent frontier enclave.
// SeatX/SeatY locate the realm's leading hall — the capital for the
// crown, the eldest hall for an enclave — and the realm takes that
// hall's name (the hall stands for the realm, as halls do).
type Realm struct {
	ID           int64
	Name         string
	IsCrown      bool
	SeatX, SeatY int64
}

// TerritoryCell assigns one land cell to a realm's sphere of control —
// the realm whose seat can reach it cheapest, within patrol range.
// Cells beyond every realm's reach are unclaimed wilds and have no row.
type TerritoryCell struct {
	X, Y    int64
	RealmID int64
}

// LakeInfo names a lake — one entry per connected cluster of RegionLake
// cells. X, Y is a representative cell (lex-smallest in the cluster) so
// downstream consumers can highlight the lake on a map.
//
// SurfaceElev is the water surface — the basin's spill level from
// pit-fill, i.e., the elevation at which the lake overflows into its
// outlet river. MaxDepth is the deepest submerged point below that
// surface. Both in meters.
type LakeInfo struct {
	ID          int64
	Name        string
	X, Y        int64
	SurfaceElev float64
	MaxDepth    float64
}

// PassInfo names a mountain pass — a saddle in the ridge that bridges
// the cradle to the plateau. One entry per RegionPass cell; the
// detection guarantees passes don't cluster.
type PassInfo struct {
	ID   int64
	Name string
	X, Y int64
}

// DenInfo names a dragon den — a mountain cell at strict local
// elevation max. Lore: dragons den in mountain caves, "high,
// defensible, hard to reach without flight." Each seed rolls a
// sparse population of dens, spaced by territory.
type DenInfo struct {
	ID        int64
	Name      string
	X, Y      int64
	Elevation float64
}

// NestInfo names a drake nest — a foothill cell at strict local
// elevation max. Lore: drakes "den lower and more variably — caves
// at the foothill level." More numerous than dragon dens; min-sep
// is half (4 cells / ~200km) to reflect drakes being "the everyday
// menace" rather than the rare apex.
type NestInfo struct {
	ID        int64
	Name      string
	X, Y      int64
	Elevation float64
}

// RookeryInfo names a wyvern rookery — a cliff cell at strict local
// elevation max. Lore: wyverns "nest like raptors — cliffs,
// rookeries, mountain spires. Often colonial." Min-sep is small
// (3 cells / ~150km) because wyverns are the densest of the
// dragon family — skirmisher-flavored, numerous.
type RookeryInfo struct {
	ID        int64
	Name      string
	X, Y      int64
	Elevation float64
}

// Road is one trade route — from a non-Tributary seat to its nearest
// Tributary, traced via Dijkstra over terrain. Rivers serve as the
// inter-Tributary spine implicitly (the lore: "the river physically
// connects them"); roads are the overland complement.
type Road struct {
	ID           int64
	FromX, FromY int64
	ToX, ToY     int64
}

// RoadCell is one cell along a Road. Ord increases from the source
// seat (FromX/FromY) toward the Tributary endpoint.
type RoadCell struct {
	RoadID int64
	X, Y   int64
	Ord    int64
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

	Regions   []RegionCell
	RiverInfo []River         // (id, name) — populated alongside Rivers
	Rivers    []RiverCell     // (river_id, x, y, ord)
	Seats     []NamedSeat     // settlements (Tributary, March, Headwater, Reach, Outhold)
	Lakes     []LakeInfo      // named lake clusters (one per connected component)
	Passes    []PassInfo      // mountain passes through the ridge
	Roads     []Road          // trade routes from each non-Tributary seat
	RoadCells []RoadCell      // cells along each road
	Dens      []DenInfo       // dragon dens at mountain peaks
	Nests     []NestInfo      // drake nests at foothill peaks
	Rookeries []RookeryInfo   // wyvern rookeries on cliffs
	Volcanoes []VolcanoInfo   // born vents on the rift shoulder
	Realms    []Realm         // polities: the Crown + independent enclaves
	Territory []TerritoryCell // realm sphere-of-control per claimed land cell

	// The seed's full volcanic timeline — every site (born or not)
	// and the entire eruption schedule. Unexported working state, not
	// hashed or persisted: feature placement reads it to keep lairs
	// and passes off ground that will one day split open, and the
	// slice simulation replays its own millennium of it live.
	volcanoSites []volcanoSite
	volcanoSched []eruption
}
