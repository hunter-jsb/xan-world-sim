-- name: ListRegions :many
SELECT * FROM regions ORDER BY id;

-- name: GetRegion :one
SELECT * FROM regions WHERE id = ?;

-- name: GetCellsInBounds :many
SELECT rc.x, rc.y, r.kind, r.name, rc.elevation
FROM region_cells rc
JOIN regions r ON r.id = rc.region_id
WHERE rc.x >= ? AND rc.x <= ? AND rc.y >= ? AND rc.y <= ?;

-- name: InsertRegion :one
INSERT INTO regions (name, kind) VALUES (?, ?) RETURNING *;

-- name: InsertCell :exec
INSERT INTO region_cells (region_id, x, y) VALUES (?, ?, ?);
