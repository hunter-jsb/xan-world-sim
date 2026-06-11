-- +goose Up
-- Basin-overflow hydrology: lakes now carry their water surface (the
-- basin's spill level from pit-fill) and deepest point. Both meters.
ALTER TABLE lakes ADD COLUMN surface_elev REAL NOT NULL DEFAULT 0;
ALTER TABLE lakes ADD COLUMN max_depth REAL NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE lakes DROP COLUMN surface_elev;
ALTER TABLE lakes DROP COLUMN max_depth;
