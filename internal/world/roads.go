package world

import "github.com/hunterjsb/xan-world-sim/internal/pqueue"

// roadItem is one frontier entry in the road-network Dijkstra. The
// tiebreaker on equal distance is (Y, X) — gives the search a stable
// expansion order so determinism is preserved across runs.
type roadItem struct {
	X, Y int
	Dist int
}

func roadItemLess(a, b roadItem) bool {
	if a.Dist != b.Dist {
		return a.Dist < b.Dist
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// buildRoads traces overland trade routes from each non-Tributary seat
// back to its nearest Tributary. The lore grounds the inter-Tributary
// network in the rivers themselves ("the river physically connects
// them — and that bond is real"); these roads complement that with the
// overland paths March / Headwater / Reach / Outhold seats need to
// plug into the heartland.
//
// Multi-source Dijkstra: all Tributaries seed the search at dist 0,
// edges weighted by terrain via roadBuildCost (rivers override to 1 —
// the cheapest going there is).
func (w *World) buildRoads() {
	hasTrib := false
	for _, s := range w.Seats {
		if s.Tier == RegionSeat {
			hasTrib = true
			break
		}
	}
	if !hasTrib {
		return
	}
	g := gridOf(w.Regions)
	riverAt := make(map[[2]int]bool, len(w.Rivers))
	for _, r := range w.Rivers {
		riverAt[[2]int{int(r.X), int(r.Y)}] = true
	}
	cost := func(x, y int) int {
		if riverAt[[2]int{x, y}] {
			return 1
		}
		return roadBuildCost(g.regionAt([2]int{x, y}))
	}

	const inf = 1 << 30
	dist := make([][]int, Height)
	parent := make([][][2]int, Height)
	for y := 0; y < Height; y++ {
		dist[y] = make([]int, Width)
		parent[y] = make([][2]int, Width)
		for x := 0; x < Width; x++ {
			dist[y][x] = inf
			parent[y][x] = [2]int{-1, -1}
		}
	}
	pq := pqueue.New(roadItemLess)
	for _, s := range w.Seats {
		if s.Tier != RegionSeat {
			continue
		}
		dist[s.Y][s.X] = 0
		pq.Push(roadItem{X: int(s.X), Y: int(s.Y), Dist: 0})
	}
	for pq.Len() > 0 {
		cur := pq.Pop()
		if cur.Dist > dist[cur.Y][cur.X] {
			continue
		}
		for _, d := range dirs8 {
			nx, ny := cur.X+d[0], cur.Y+d[1]
			if !inBounds(nx, ny) {
				continue
			}
			c := cost(nx, ny)
			if c < 0 {
				continue
			}
			newDist := cur.Dist + c
			if newDist < dist[ny][nx] {
				dist[ny][nx] = newDist
				parent[ny][nx] = [2]int{cur.X, cur.Y}
				pq.Push(roadItem{X: nx, Y: ny, Dist: newDist})
			}
		}
	}
	// For each non-Tributary seat, walk parent[] back to source.
	var nextRoadID int64 = 1
	for _, s := range w.Seats {
		if s.Tier == RegionSeat {
			continue
		}
		sx, sy := int(s.X), int(s.Y)
		if dist[sy][sx] == inf {
			continue // unreachable
		}
		var path [][2]int
		cx, cy := sx, sy
		path = append(path, [2]int{cx, cy})
		for {
			if dist[cy][cx] == 0 {
				break // reached a Tributary
			}
			p := parent[cy][cx]
			if p[0] < 0 {
				break
			}
			path = append(path, p)
			cx, cy = p[0], p[1]
			if len(path) > Width*Height {
				break // safety
			}
		}
		if len(path) < 2 {
			continue
		}
		toX, toY := int64(path[len(path)-1][0]), int64(path[len(path)-1][1])
		w.Roads = append(w.Roads, Road{
			ID:    nextRoadID,
			FromX: s.X, FromY: s.Y,
			ToX: toX, ToY: toY,
		})
		for i, c := range path {
			w.RoadCells = append(w.RoadCells, RoadCell{
				RoadID: nextRoadID,
				X:      int64(c[0]),
				Y:      int64(c[1]),
				Ord:    int64(i + 1),
			})
		}
		nextRoadID++
	}
}
