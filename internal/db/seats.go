package db

import "context"

type GetSeatsInBoundsRow struct {
	X        int64   `json:"x"`
	Y        int64   `json:"y"`
	Tier     string  `json:"tier"`
	Name     string  `json:"name"`
	Pressure float64 `json:"pressure"`
}

const getSeatsInBounds = `
SELECT s.x, s.y, r.kind AS tier, s.name, s.pressure
FROM seats s
JOIN regions r ON r.id = s.tier_id
WHERE s.x >= ? AND s.x <= ? AND s.y >= ? AND s.y <= ?
`

func (q *Queries) GetSeatsInBounds(ctx context.Context, minX, maxX, minY, maxY int64) ([]GetSeatsInBoundsRow, error) {
	rows, err := q.db.QueryContext(ctx, getSeatsInBounds, minX, maxX, minY, maxY)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GetSeatsInBoundsRow
	for rows.Next() {
		var r GetSeatsInBoundsRow
		if err := rows.Scan(&r.X, &r.Y, &r.Tier, &r.Name, &r.Pressure); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
