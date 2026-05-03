-- +goose Up
-- +goose StatementBegin
-- First biome refinement: cradle cells split into forest (cool
-- temperate) and tundra (cold) based on cell temperature, with the
-- default cradle kind reserved for warm temperate / Mediterranean
-- grassland conditions. Foothills stay as foothills regardless of
-- biome — they're a distinct topographic identity, not a vegetation
-- one.
INSERT INTO regions (id, name, kind) VALUES
    (15, 'Forest',  'forest'),
    (16, 'Tundra',  'tundra');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id IN (15, 16);
-- +goose StatementEnd
