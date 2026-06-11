package db

import "context"

type GetSeatsInBoundsRow struct {
	X          int64   `json:"x"`
	Y          int64   `json:"y"`
	Tier       string  `json:"tier"`
	Name       string  `json:"name"`
	Pressure   float64 `json:"pressure"`
	Allegiance float64 `json:"allegiance"`
	RealmID    int64   `json:"realm_id"`   // 0 = no realm (e.g., LGM)
	RealmName  string  `json:"realm_name"` // "" = no realm
	IsCrown    bool    `json:"is_crown"`
}

const getSeatsInBounds = `
SELECT s.x, s.y, r.kind AS tier, s.name, s.pressure, s.allegiance,
       s.realm_id,
       COALESCE(rm.name, '') AS realm_name,
       COALESCE(rm.is_crown, 0) AS is_crown
FROM seats s
JOIN regions r ON r.id = s.tier_id
LEFT JOIN realms rm ON rm.id = s.realm_id
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
		if err := rows.Scan(&r.X, &r.Y, &r.Tier, &r.Name, &r.Pressure,
			&r.Allegiance, &r.RealmID, &r.RealmName, &r.IsCrown); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
