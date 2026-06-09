package db

import "context"

type GetNamedFeaturesInBoundsRow struct {
	X    int64  `json:"x"`
	Y    int64  `json:"y"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// getNamedFeaturesInBounds unions dens, drake nests, wyvern rookeries,
// lakes, and passes — any named feature that has a procgen name stored
// separately from the region_cells kind field.
const getNamedFeaturesInBounds = `
SELECT x, y, 'den'     AS kind, name FROM dens
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'nest'    AS kind, name FROM drake_nests
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'rookery' AS kind, name FROM wyvern_rookeries
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'lake'    AS kind, name FROM lakes
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'pass'    AS kind, name FROM passes
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
`

func (q *Queries) GetNamedFeaturesInBounds(ctx context.Context, minX, maxX, minY, maxY int64) ([]GetNamedFeaturesInBoundsRow, error) {
	rows, err := q.db.QueryContext(ctx, getNamedFeaturesInBounds,
		minX, maxX, minY, maxY,
		minX, maxX, minY, maxY,
		minX, maxX, minY, maxY,
		minX, maxX, minY, maxY,
		minX, maxX, minY, maxY,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GetNamedFeaturesInBoundsRow
	for rows.Next() {
		var r GetNamedFeaturesInBoundsRow
		if err := rows.Scan(&r.X, &r.Y, &r.Kind, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
