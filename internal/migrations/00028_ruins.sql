-- +goose Up
-- Ruined halls: seats sacked during a simulation slice. The generator
-- never emits this region (deep time's snapshots hold only living
-- halls); it exists so the sim's overlay rows and the renderer agree
-- on the kind.
INSERT INTO regions (id, name, kind) VALUES (28, 'Ruined Hall', 'ruin');

-- +goose Down
DELETE FROM regions WHERE id = 28;
