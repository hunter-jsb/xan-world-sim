-- +goose Up
-- +goose StatementBegin
-- One row per named lake (a connected cluster of RegionLake cells).
-- The (x, y) is a representative cell — lex-smallest in the cluster —
-- so callers can highlight the lake on the map. The actual cell set
-- lives in region_cells with kind='lake'.
CREATE TABLE lakes (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    x    INTEGER NOT NULL,
    y    INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS lakes;
-- +goose StatementEnd
