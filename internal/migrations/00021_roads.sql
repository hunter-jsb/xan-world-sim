-- +goose Up
-- +goose StatementBegin
-- Roads: trade routes connecting non-Tributary seats back to their
-- nearest Tributary. The lore explicitly grounds the inter-Tributary
-- network in rivers ("the river physically connects them — and that
-- bond is real"); these tables capture the *overland* complement
-- that brings Marches, Headwater holds, Reaches, and Outholds into
-- the trade network.
CREATE TABLE roads (
    id     INTEGER PRIMARY KEY,
    from_x INTEGER NOT NULL,
    from_y INTEGER NOT NULL,
    to_x   INTEGER NOT NULL,
    to_y   INTEGER NOT NULL
);

CREATE TABLE road_cells (
    road_id INTEGER NOT NULL REFERENCES roads(id) ON DELETE CASCADE,
    x       INTEGER NOT NULL,
    y       INTEGER NOT NULL,
    ord     INTEGER NOT NULL,
    PRIMARY KEY (road_id, ord)
);

CREATE INDEX idx_road_cells_xy ON road_cells(x, y);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS road_cells;
DROP TABLE IF EXISTS roads;
-- +goose StatementEnd
