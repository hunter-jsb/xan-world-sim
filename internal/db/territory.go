package db

import "context"

// GetTerritoryInBoundsRow is one claimed land cell with its owning
// realm, for the political map view and info panel. Contested is
// never set by the query — deep time's borders are settled; only the
// simulation overlay marks contested marchland.
type GetTerritoryInBoundsRow struct {
	X         int64  `json:"x"`
	Y         int64  `json:"y"`
	RealmID   int64  `json:"realm_id"`
	RealmName string `json:"realm_name"`
	IsCrown   bool   `json:"is_crown"`
	RealmAge  int64  `json:"realm_age"`
	Contested bool   `json:"contested"`
}

const getTerritoryInBounds = `
SELECT t.x, t.y, t.realm_id, rm.name, rm.is_crown, rm.age
FROM territory t
JOIN realms rm ON rm.id = t.realm_id
WHERE t.x >= ? AND t.x <= ? AND t.y >= ? AND t.y <= ?
`

func (q *Queries) GetTerritoryInBounds(ctx context.Context, minX, maxX, minY, maxY int64) ([]GetTerritoryInBoundsRow, error) {
	rows, err := q.db.QueryContext(ctx, getTerritoryInBounds, minX, maxX, minY, maxY)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GetTerritoryInBoundsRow
	for rows.Next() {
		var r GetTerritoryInBoundsRow
		if err := rows.Scan(&r.X, &r.Y, &r.RealmID, &r.RealmName, &r.IsCrown, &r.RealmAge); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
