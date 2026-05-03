package db

import "context"

// GetRoadCellsInBoundsRow is one road cell within a viewport. Mirrors
// the sqlc-generated rivers row so the renderer can treat them similarly.
type GetRoadCellsInBoundsRow struct {
	RoadID int64 `json:"road_id"`
	X      int64 `json:"x"`
	Y      int64 `json:"y"`
	Ord    int64 `json:"ord"`
}

const getRoadCellsInBounds = `
SELECT road_id, x, y, ord
FROM road_cells
WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
ORDER BY road_id, ord
`

// GetRoadCellsInBounds returns all road cells within the inclusive
// viewport. Hand-written rather than sqlc-generated to keep this PR
// self-contained — the schema lives in migration 00021.
func (q *Queries) GetRoadCellsInBounds(ctx context.Context, minX, maxX, minY, maxY int64) ([]GetRoadCellsInBoundsRow, error) {
	rows, err := q.db.QueryContext(ctx, getRoadCellsInBounds, minX, maxX, minY, maxY)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GetRoadCellsInBoundsRow{}
	for rows.Next() {
		var r GetRoadCellsInBoundsRow
		if err := rows.Scan(&r.RoadID, &r.X, &r.Y, &r.Ord); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
