-- +goose Up
-- +goose StatementBegin
-- Lakes are emergent: cells where pit-fill detects a basin floor
-- (flow direction routes to a neighbor higher in bedrock terms).
-- Computed at generate time, persisted as a region kind for the
-- renderer.
INSERT INTO regions (id, name, kind) VALUES
    (14, 'Lakes', 'lake');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 14;
-- +goose StatementEnd
