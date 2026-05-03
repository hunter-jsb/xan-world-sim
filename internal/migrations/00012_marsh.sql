-- +goose Up
-- +goose StatementBegin
-- Marsh: the wet biome. Vegetated lowland directly adjacent to a
-- water body (river, lake, sea, brine) where the cell temperature
-- is above freezing. Detected as a post-process during Generate.
INSERT INTO regions (id, name, kind) VALUES
    (17, 'Marsh', 'marsh');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 17;
-- +goose StatementEnd
