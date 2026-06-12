-- +goose Up
-- Living geology. Bedrock now carries a history (uplift, volcanism,
-- ice, isostasy, erosion integrated from 600 kya to the world's
-- moment), so region_cells gains the topmost lithology and its age,
-- and the rift's volcanoes get a roster. This also supersedes
-- 00029's note: drainage is derived from the *evolved* bedrock, so
-- it shifts with kya as lava dams and moraines reroute the flow.
--
-- rock: 1 basement shield, 2 orogenic rock, 3 marine sediment,
-- 4 alluvium, 5 glacial till, 6 loess, 7 volcanic rock — mirrored in
-- world/geology.go (Rock* constants) and render.RockColor.
-- rock_age: ka before the world's moment the surface was laid.
INSERT INTO regions (id, name, kind) VALUES (29, 'Volcano', 'volcano');
INSERT INTO regions (id, name, kind) VALUES (30, 'Lava Field', 'lava');
ALTER TABLE region_cells ADD COLUMN rock INTEGER NOT NULL DEFAULT 0;
ALTER TABLE region_cells ADD COLUMN rock_age INTEGER NOT NULL DEFAULT 0;

-- last_eruption_ago / eruptions are relative to the world's moment:
-- a vent only has a row once it has erupted at least once by then.
CREATE TABLE volcanoes (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    x INTEGER NOT NULL,
    y INTEGER NOT NULL,
    elevation REAL NOT NULL,
    last_eruption_ago INTEGER NOT NULL,
    eruptions INTEGER NOT NULL
);

-- +goose Down
DROP TABLE volcanoes;
ALTER TABLE region_cells DROP COLUMN rock_age;
ALTER TABLE region_cells DROP COLUMN rock;
DELETE FROM regions WHERE id IN (29, 30);
