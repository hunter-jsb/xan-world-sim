package db

import "context"

type GetNamedFeaturesInBoundsRow struct {
	X    int64  `json:"x"`
	Y    int64  `json:"y"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Detail is an optional human-readable annotation for the info
	// panel — currently only lakes carry one (bathymetry).
	Detail string `json:"detail"`
}

// getNamedFeaturesInBounds unions dens, drake nests, wyvern rookeries,
// lakes, passes, and volcanoes — any named feature that has a procgen
// name stored separately from the region_cells kind field.
const getNamedFeaturesInBounds = `
SELECT x, y, 'den'     AS kind, name, '' AS detail FROM dens
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'nest'    AS kind, name, '' AS detail FROM drake_nests
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'rookery' AS kind, name, '' AS detail FROM wyvern_rookeries
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'lake'    AS kind, name,
       printf('surface %dm, depth %dm',
              CAST(round(surface_elev) AS INTEGER),
              CAST(round(max_depth) AS INTEGER)) AS detail
  FROM lakes
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'pass'    AS kind, name, '' AS detail FROM passes
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
UNION ALL
SELECT x, y, 'volcano' AS kind, name,
       CASE WHEN last_eruption_ago = 0 THEN 'erupting now'
            ELSE printf('last erupted %d ka ago', last_eruption_ago)
       END AS detail
  FROM volcanoes
  WHERE x >= ? AND x <= ? AND y >= ? AND y <= ?
`

func (q *Queries) GetNamedFeaturesInBounds(ctx context.Context, minX, maxX, minY, maxY int64) ([]GetNamedFeaturesInBoundsRow, error) {
	rows, err := q.db.QueryContext(ctx, getNamedFeaturesInBounds,
		minX, maxX, minY, maxY,
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
		if err := rows.Scan(&r.X, &r.Y, &r.Kind, &r.Name, &r.Detail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
