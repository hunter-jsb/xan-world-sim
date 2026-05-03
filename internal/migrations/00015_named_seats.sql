-- +goose Up
-- +goose StatementBegin
-- A seat is a settlement cell with a generated name. The cell's
-- region (in region_cells) carries the kind for rendering; this
-- table carries the proper-noun identity. PK on (x,y) so the seat is
-- uniquely tied to its location.
CREATE TABLE seats (
    x       INTEGER NOT NULL,
    y       INTEGER NOT NULL,
    tier_id INTEGER NOT NULL,
    name    TEXT    NOT NULL,
    PRIMARY KEY (x, y),
    FOREIGN KEY (tier_id) REFERENCES regions(id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS seats;
-- +goose StatementEnd
