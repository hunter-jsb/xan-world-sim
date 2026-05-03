-- +goose Up
-- +goose StatementBegin
-- Dragon dens — explicit procgen target from the dragons lore: "the
-- Mountain Barrier is dotted with dragon-dens of varying scale."
-- Detected as mountain cells at strict local elevation maxima in
-- 5x5 windows, with greedy spatial dedup at min-sep 6 cells (~300km
-- of territory at our cell size, the scale of a dragon's hunting
-- range as implied by the lore's "raid radius").
INSERT INTO regions (id, name, kind) VALUES
    (24, 'Dragon Den', 'den');

CREATE TABLE dens (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    x         INTEGER NOT NULL,
    y         INTEGER NOT NULL,
    elevation REAL NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 24;
DROP TABLE IF EXISTS dens;
-- +goose StatementEnd
