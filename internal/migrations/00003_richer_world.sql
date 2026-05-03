-- +goose Up
-- +goose StatementBegin

-- Schema: rivers
CREATE TABLE rivers (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE river_cells (
    river_id INTEGER NOT NULL REFERENCES rivers(id) ON DELETE CASCADE,
    x        INTEGER NOT NULL,
    y        INTEGER NOT NULL,
    ord      INTEGER NOT NULL,
    PRIMARY KEY (river_id, ord)
);

CREATE INDEX idx_river_cells_xy ON river_cells(x, y);

-- Add the Doab as its own region (mountainous wedge inside the cradle)
INSERT INTO regions (id, name, kind) VALUES (8, 'The Doab', 'doab');

-- Wipe placeholder geometry from migration 2; lay down a 60x22 sketch.
DELETE FROM region_cells;

-- Plateau across the full width (y=0..2)
WITH RECURSIVE
  xs(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM xs WHERE x < 59),
  ys(y) AS (VALUES(0) UNION ALL SELECT y+1 FROM ys WHERE y < 2)
INSERT INTO region_cells (region_id, x, y) SELECT 1, x, y FROM xs, ys;

-- Mountain Barrier (y=3)
WITH RECURSIVE xs(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM xs WHERE x < 59)
INSERT INTO region_cells (region_id, x, y) SELECT 2, x, 3 FROM xs;

-- Brine: western strip (x=0..1, y=4..21)
WITH RECURSIVE
  xs(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM xs WHERE x < 1),
  ys(y) AS (VALUES(4) UNION ALL SELECT y+1 FROM ys WHERE y < 21)
INSERT INTO region_cells (region_id, x, y) SELECT 4, x, y FROM xs, ys;

-- Eastern Sea: eastern strip (x=52..59, y=4..21)
WITH RECURSIVE
  xs(x) AS (VALUES(52) UNION ALL SELECT x+1 FROM xs WHERE x < 59),
  ys(y) AS (VALUES(4) UNION ALL SELECT y+1 FROM ys WHERE y < 21)
INSERT INTO region_cells (region_id, x, y) SELECT 5, x, y FROM xs, ys;

-- Cradle (x=2..51, y=4..21) EXCEPT the doab area
WITH RECURSIVE
  xs(x) AS (VALUES(2) UNION ALL SELECT x+1 FROM xs WHERE x < 51),
  ys(y) AS (VALUES(4) UNION ALL SELECT y+1 FROM ys WHERE y < 21)
INSERT INTO region_cells (region_id, x, y)
SELECT 3, x, y FROM xs, ys
WHERE NOT (x BETWEEN 13 AND 15 AND y BETWEEN 4 AND 7);

-- Doab: mountainous wedge between the two northern rivers (x=13..15, y=4..7)
WITH RECURSIVE
  xs(x) AS (VALUES(13) UNION ALL SELECT x+1 FROM xs WHERE x < 15),
  ys(y) AS (VALUES(4) UNION ALL SELECT y+1 FROM ys WHERE y < 7)
INSERT INTO region_cells (region_id, x, y) SELECT 8, x, y FROM xs, ys;

-- Rivers
INSERT INTO rivers (id, name) VALUES
    (1, 'Northern West Fork'),
    (2, 'Northern East Fork'),
    (3, 'Combined Northern River'),
    (4, 'Southern Feeder'),
    (5, 'Main River');

-- Northern West Fork: born at (12,4), flows south, rounds the doab west, confluence at (14,8)
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (1,12,4,1),(1,12,5,2),(1,12,6,3),(1,12,7,4),(1,13,8,5),(1,14,8,6);

-- Northern East Fork: born at (16,4), flows south, rounds the doab east, confluence at (14,8)
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (2,16,4,1),(2,16,5,2),(2,16,6,3),(2,16,7,4),(2,15,8,5),(2,14,8,6);

-- Combined Northern River: from confluence (14,8) flowing southeast to (20,15)
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (3,14,9,1),(3,15,10,2),(3,16,11,3),(3,17,12,4),(3,18,13,5),(3,19,14,6),(3,20,15,7);

-- Southern Feeder: enters from south at (14,21), flows north to merge at (20,15)
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (4,14,21,1),(4,14,20,2),(4,15,19,3),(4,16,18,4),(4,17,17,5),(4,18,16,6),(4,19,15,7),(4,20,15,8);

-- Main River: from merge (20,15) flowing east-southeast to exit at (51,18) into the Eastern Sea
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (5,21,15,1),(5,22,15,2),(5,23,16,3),(5,24,16,4),(5,25,16,5),
    (5,26,17,6),(5,27,17,7),(5,28,17,8),(5,29,17,9),
    (5,30,18,10),(5,31,18,11),(5,32,18,12),(5,33,18,13),(5,34,18,14),
    (5,35,18,15),(5,36,18,16),(5,37,18,17),(5,38,18,18),(5,39,18,19),
    (5,40,18,20),(5,41,18,21),(5,42,18,22),(5,43,18,23),(5,44,18,24),
    (5,45,18,25),(5,46,18,26),(5,47,18,27),(5,48,18,28),(5,49,18,29),
    (5,50,18,30),(5,51,18,31);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM region_cells;
DELETE FROM regions WHERE id = 8;
DROP INDEX IF EXISTS idx_river_cells_xy;
DROP TABLE IF EXISTS river_cells;
DROP TABLE IF EXISTS rivers;
-- +goose StatementEnd
