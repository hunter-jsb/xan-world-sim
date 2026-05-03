-- +goose Up
-- +goose StatementBegin
-- Drake nests — the lesser cousin of dragon dens. From `dragons.md`:
-- "Drakes — the everyday menace. Smaller, four-legged or bipedal,
-- more bestial, less intelligent. The actual creature most northern
-- defenses are built against. Drakes den lower and more variably —
-- caves at the foothill level, abandoned ruins, deep forest."
-- Detected as foothill cells at strict local elevation max in 5x5
-- windows, greedy dedup at min-sep 4 cells (~200km, half the
-- dragon-territory radius — drakes are more numerous).
INSERT INTO regions (id, name, kind) VALUES
    (25, 'Drake Nest', 'nest');

CREATE TABLE drake_nests (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    x         INTEGER NOT NULL,
    y         INTEGER NOT NULL,
    elevation REAL NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 25;
DROP TABLE IF EXISTS drake_nests;
-- +goose StatementEnd
