-- +goose Up
-- +goose StatementBegin
-- Mountain passes — saddles through the ridge that connect the
-- cradle to the plateau. Detected as mountain cells at locally
-- minimal elevation among their mountain neighbors *and* with a
-- foothill/cradle south approach. From the lore: "pre-Melt these
-- were passable; the Melt made them spectacular and brutal."
INSERT INTO regions (id, name, kind) VALUES
    (23, 'Mountain Pass', 'pass');

CREATE TABLE passes (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    x    INTEGER NOT NULL,
    y    INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 23;
DROP TABLE IF EXISTS passes;
-- +goose StatementEnd
