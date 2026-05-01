-- +goose Up
-- +goose StatementBegin
CREATE TABLE regions (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL
);

CREATE TABLE region_cells (
    region_id INTEGER NOT NULL REFERENCES regions(id) ON DELETE CASCADE,
    x         INTEGER NOT NULL,
    y         INTEGER NOT NULL,
    PRIMARY KEY (region_id, x, y)
);

CREATE INDEX idx_region_cells_xy ON region_cells(x, y);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_region_cells_xy;
DROP TABLE IF EXISTS region_cells;
DROP TABLE IF EXISTS regions;
-- +goose StatementEnd
