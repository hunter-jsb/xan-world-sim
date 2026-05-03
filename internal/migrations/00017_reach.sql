-- +goose Up
-- +goose StatementBegin
-- Reach — the frontier-explorer seat tier. From the lore: "a seat at
-- the far edge of crown reach, so remote it is essentially autonomous
-- in practice. Crown couriers arrive late or never." Detected as the
-- land cells maximally distant from the Tributary centroid (the
-- heartland's center of gravity), with spatial dedup so different
-- Reaches sit in different cardinal directions.
INSERT INTO regions (id, name, kind) VALUES
    (22, 'Reach', 'reach');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 22;
-- +goose StatementEnd
