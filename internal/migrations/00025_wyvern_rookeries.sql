-- +goose Up
-- +goose StatementBegin
-- Wyvern rookeries — third tier of the dragon-family lore:
-- "Wyvern — the lesser raider. Bipedal, two wings (no front legs),
-- poison-tailed. Numerous, skirmisher-flavored." Nests "like raptors
-- — cliffs, rookeries, mountain spires. Often colonial." Detected
-- as cliff cells at strict local elevation max in 3x3 windows, with
-- min-sep 3 cells (~150km — densest of the trio since wyverns are
-- the most numerous).
INSERT INTO regions (id, name, kind) VALUES
    (26, 'Wyvern Rookery', 'rookery');

CREATE TABLE wyvern_rookeries (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    x         INTEGER NOT NULL,
    y         INTEGER NOT NULL,
    elevation REAL NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 26;
DROP TABLE IF EXISTS wyvern_rookeries;
-- +goose StatementEnd
