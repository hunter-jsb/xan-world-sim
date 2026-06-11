package world

import (
	"reflect"
	"testing"
)

func TestGridOf_Lookups(t *testing.T) {
	regions := []RegionCell{
		{RegionID: RegionCradle, X: 3, Y: 4, Elevation: 120},
		{RegionID: RegionMountain, X: 5, Y: 1, Elevation: 3100},
	}
	g := gridOf(regions)
	if got := g.regionAt([2]int{3, 4}); got != RegionCradle {
		t.Errorf("regionAt(3,4) = %d, want %d", got, RegionCradle)
	}
	if got := g.elevAt([2]int{5, 1}); got != 3100 {
		t.Errorf("elevAt(5,1) = %g, want 3100", got)
	}
	// Missing cells read as zero values — stages rely on this.
	if got := g.regionAt([2]int{9, 9}); got != 0 {
		t.Errorf("regionAt(missing) = %d, want 0", got)
	}
	if got := g.elevAt([2]int{9, 9}); got != 0 {
		t.Errorf("elevAt(missing) = %g, want 0", got)
	}
}

func TestInBounds(t *testing.T) {
	cases := []struct {
		x, y int
		want bool
	}{
		{0, 0, true},
		{Width - 1, Height - 1, true},
		{-1, 0, false},
		{0, -1, false},
		{Width, 0, false},
		{0, Height, false},
	}
	for _, c := range cases {
		if got := inBounds(c.x, c.y); got != c.want {
			t.Errorf("inBounds(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestSortYX(t *testing.T) {
	pts := [][2]int{{5, 2}, {1, 1}, {0, 2}, {3, 1}}
	sortYX(pts)
	want := [][2]int{{1, 1}, {3, 1}, {0, 2}, {5, 2}}
	if !reflect.DeepEqual(pts, want) {
		t.Errorf("sortYX = %v, want %v", pts, want)
	}

	pts64 := [][2]int64{{7, 0}, {2, 3}, {1, 0}}
	sortYX(pts64)
	want64 := [][2]int64{{1, 0}, {7, 0}, {2, 3}}
	if !reflect.DeepEqual(pts64, want64) {
		t.Errorf("sortYX(int64) = %v, want %v", pts64, want64)
	}
}

func TestComponents_SplitsClusters(t *testing.T) {
	// Two 8-connected clusters: an L at (1,1) and a pair at (5,5)/(6,6)
	// (diagonal adjacency counts).
	member := map[[2]int]bool{
		{1, 1}: true, {2, 1}: true, {1, 2}: true,
		{5, 5}: true, {6, 6}: true,
	}
	var seeds [][2]int
	for p := range member {
		seeds = append(seeds, p)
	}
	sortYX(seeds)
	comps := components(seeds, func(p [2]int) bool { return member[p] })
	if len(comps) != 2 {
		t.Fatalf("got %d components, want 2", len(comps))
	}
	if len(comps[0]) != 3 || len(comps[1]) != 2 {
		t.Fatalf("component sizes = %d, %d; want 3, 2", len(comps[0]), len(comps[1]))
	}
}

func TestComponents_BFSOrder(t *testing.T) {
	// Callers resolve first-found ties using BFS discovery order:
	// seed first, then neighbors in dirs8 (row-major) order. Pin it.
	member := map[[2]int]bool{
		{1, 1}: true, {2, 1}: true, {1, 2}: true,
	}
	comps := components([][2]int{{1, 1}}, func(p [2]int) bool { return member[p] })
	if len(comps) != 1 {
		t.Fatalf("got %d components, want 1", len(comps))
	}
	want := [][2]int{{1, 1}, {2, 1}, {1, 2}}
	if !reflect.DeepEqual(comps[0], want) {
		t.Errorf("BFS order = %v, want %v", comps[0], want)
	}
}

func TestComponents_SkipsNonMemberSeeds(t *testing.T) {
	member := map[[2]int]bool{{3, 3}: true}
	comps := components([][2]int{{0, 0}, {3, 3}}, func(p [2]int) bool { return member[p] })
	if len(comps) != 1 || len(comps[0]) != 1 || comps[0][0] != [2]int{3, 3} {
		t.Errorf("components = %v, want [[{3,3}]]", comps)
	}
}
