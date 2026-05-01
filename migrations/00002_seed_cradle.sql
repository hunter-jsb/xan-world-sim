-- +goose Up
-- +goose StatementBegin
INSERT INTO regions (id, name, kind) VALUES
    (1, 'The Plateau',     'plateau'),
    (2, 'Mountain Barrier','mountain'),
    (3, 'The Cradle',      'cradle'),
    (4, 'The Brine',       'sea_brine'),
    (5, 'The Eastern Sea', 'sea_eastern'),
    (6, 'Unknown Lands',   'unknown'),
    (7, 'Agraria (drowned)','drowned');

-- placeholder geometry: a tiny hand-laid sketch just to prove the pipeline.
-- coords are (x,y) on a small grid; we'll grow this when we have a real renderer.
-- y increases downward (north -> south).

-- Plateau across the top (y=0..2)
INSERT INTO region_cells (region_id, x, y) VALUES
    (1,0,0),(1,1,0),(1,2,0),(1,3,0),(1,4,0),(1,5,0),(1,6,0),(1,7,0),(1,8,0),(1,9,0),(1,10,0),
    (1,0,1),(1,1,1),(1,2,1),(1,3,1),(1,4,1),(1,5,1),(1,6,1),(1,7,1),(1,8,1),(1,9,1),(1,10,1),
    (1,0,2),(1,1,2),(1,2,2),(1,3,2),(1,4,2),(1,5,2),(1,6,2),(1,7,2),(1,8,2),(1,9,2),(1,10,2);

-- Mountain Barrier (y=3)
INSERT INTO region_cells (region_id, x, y) VALUES
    (2,0,3),(2,1,3),(2,2,3),(2,3,3),(2,4,3),(2,5,3),(2,6,3),(2,7,3),(2,8,3),(2,9,3),(2,10,3);

-- Brine on the west (x=0, y=4..7)
INSERT INTO region_cells (region_id, x, y) VALUES
    (4,0,4),(4,0,5),(4,0,6),(4,0,7);

-- Cradle in the middle (x=1..7, y=4..7)
INSERT INTO region_cells (region_id, x, y) VALUES
    (3,1,4),(3,2,4),(3,3,4),(3,4,4),(3,5,4),(3,6,4),(3,7,4),
    (3,1,5),(3,2,5),(3,3,5),(3,4,5),(3,5,5),(3,6,5),(3,7,5),
    (3,1,6),(3,2,6),(3,3,6),(3,4,6),(3,5,6),(3,6,6),(3,7,6),
    (3,1,7),(3,2,7),(3,3,7),(3,4,7),(3,5,7),(3,6,7),(3,7,7);

-- Eastern Sea (x=8..10, y=4..7)
INSERT INTO region_cells (region_id, x, y) VALUES
    (5,8,4),(5,9,4),(5,10,4),
    (5,8,5),(5,9,5),(5,10,5),
    (5,8,6),(5,9,6),(5,10,6),
    (5,8,7),(5,9,7),(5,10,7);

-- Unknown lands (south of eastern sea, y=8)
INSERT INTO region_cells (region_id, x, y) VALUES
    (6,8,8),(6,9,8),(6,10,8);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM region_cells;
DELETE FROM regions;
-- +goose StatementEnd
