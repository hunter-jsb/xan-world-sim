-- +goose Up
-- +goose StatementBegin
-- Persist per-cell bedrock elevation so the renderer can shade cells
-- by height within each kind. Default 0 lets old data round-trip; new
-- writes always carry the actual heightmap value.
ALTER TABLE region_cells ADD COLUMN elevation REAL NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE region_cells DROP COLUMN elevation;
-- +goose StatementEnd
