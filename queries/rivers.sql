-- name: ListRivers :many
SELECT * FROM rivers ORDER BY id;

-- name: GetRiverCellsInBounds :many
SELECT rc.x, rc.y, r.name AS river_name
FROM river_cells rc
JOIN rivers r ON r.id = rc.river_id
WHERE rc.x >= ? AND rc.x <= ? AND rc.y >= ? AND rc.y <= ?
ORDER BY rc.river_id, rc.ord;
