-- +goose Up
-- +goose StatementBegin

-- Reshape the Rift (plateau-cradle boundary) so its character varies along
-- its length: steep clifflands in the SW, classic mountain band in the
-- middle, asymptotic foothill blend in the NE. Two new region kinds:
-- 'cliff' and 'foothill'.

INSERT INTO regions (id, name, kind) VALUES
    (9,  'The Western Clifflands', 'cliff'),
    (10, 'The Eastern Foothills',  'foothill');

DELETE FROM river_cells;
DELETE FROM region_cells;

-- Brine: full west strip
WITH RECURSIVE
  xs(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM xs WHERE x < 1),
  ys(y) AS (VALUES(0) UNION ALL SELECT y+1 FROM ys WHERE y < 21)
INSERT INTO region_cells (region_id, x, y) SELECT 4, x, y FROM xs, ys;

-- Eastern Sea: full east strip
WITH RECURSIVE
  xs(x) AS (VALUES(52) UNION ALL SELECT x+1 FROM xs WHERE x < 59),
  ys(y) AS (VALUES(0) UNION ALL SELECT y+1 FROM ys WHERE y < 21)
INSERT INTO region_cells (region_id, x, y) SELECT 5, x, y FROM xs, ys;

-- Plateau / Cliff / Mountain / Foothill / Doab / Cradle in one CTE pass.
--
-- mrow(x) = row of the Rift at column x (the boundary itself, painted as
--           cliff in SW or mountain elsewhere).
-- ft(x)   = thickness of the foothill belt south of the Rift, in rows.
--           Zero in the SW (cliff zone), grows toward the NE.
WITH RECURSIVE
  xs(x) AS (VALUES(2) UNION ALL SELECT x+1 FROM xs WHERE x < 51),
  ys(y) AS (VALUES(0) UNION ALL SELECT y+1 FROM ys WHERE y < 21),
  cells(x, y, mrow, ft) AS (
    SELECT x, y,
      CASE
        WHEN x BETWEEN 48 AND 51 THEN 2
        WHEN x BETWEEN 44 AND 47 THEN 3
        WHEN x BETWEEN 40 AND 43 THEN 4
        WHEN x BETWEEN 36 AND 39 THEN 5
        WHEN x BETWEEN 32 AND 35 THEN 6
        WHEN x BETWEEN 28 AND 31 THEN 7
        WHEN x BETWEEN 24 AND 27 THEN 8
        WHEN x BETWEEN 20 AND 23 THEN 9
        WHEN x BETWEEN 16 AND 19 THEN 10
        WHEN x BETWEEN 12 AND 15 THEN 11
        WHEN x BETWEEN  8 AND 11 THEN 12
        WHEN x BETWEEN  4 AND  7 THEN 13
        WHEN x BETWEEN  2 AND  3 THEN 14
      END,
      CASE
        WHEN x BETWEEN  2 AND 11 THEN 0   -- SW: cliffs, no foothills
        WHEN x BETWEEN 12 AND 23 THEN 1
        WHEN x BETWEEN 24 AND 35 THEN 2
        WHEN x BETWEEN 36 AND 51 THEN 3   -- NE: thickest foothill belt
      END
    FROM xs, ys
  )
INSERT INTO region_cells (region_id, x, y)
SELECT
  CASE
    WHEN y < mrow THEN 1                                                                 -- plateau
    WHEN y = mrow AND x BETWEEN 2 AND 11 THEN 9                                          -- cliff (SW)
    WHEN y = mrow THEN 2                                                                 -- mountain
    WHEN (x BETWEEN 18 AND 21 AND y IN (11, 12)) OR (x BETWEEN 18 AND 20 AND y = 13) THEN 8  -- doab
    WHEN y > mrow AND y <= mrow + ft THEN 10                                             -- foothill
    ELSE 3                                                                                -- cradle
  END,
  x, y
FROM cells;

-- Rivers (unchanged from migration 4 — they still flow correctly through
-- the cradle band south of the Rift).
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (1,17,11,1),(1,17,12,2),(1,17,13,3),(1,18,14,4),(1,19,14,5);
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (2,22,10,1),(2,22,11,2),(2,22,12,3),(2,21,13,4),(2,20,14,5),(2,19,14,6);
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (3,20,15,1),(3,21,16,2),(3,22,17,3),(3,23,18,4),(3,24,18,5);
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (4,28,21,1),(4,27,20,2),(4,26,19,3),(4,25,18,4),(4,24,18,5);
INSERT INTO river_cells (river_id, x, y, ord) VALUES
    (5,25,18,1),(5,26,18,2),(5,27,18,3),(5,28,18,4),(5,29,18,5),
    (5,30,18,6),(5,31,18,7),(5,32,18,8),(5,33,18,9),(5,34,18,10),
    (5,35,18,11),(5,36,18,12),(5,37,18,13),(5,38,18,14),(5,39,18,15),
    (5,40,18,16),(5,41,18,17),(5,42,18,18),(5,43,18,19),(5,44,18,20),
    (5,45,18,21),(5,46,18,22),(5,47,18,23),(5,48,18,24),(5,49,18,25),
    (5,50,18,26),(5,51,18,27);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM river_cells;
DELETE FROM region_cells;
DELETE FROM regions WHERE id IN (9, 10);
-- +goose StatementEnd
