-- +goose Up
-- +goose StatementBegin
-- Outhold — the catch-all seat tier from the lore. Off-river,
-- off-grid, no formal frontier role. Ranchers, prospectors,
-- religious oddballs, fugitives. Detected as land cells at strict
-- local maxima of distance-from-civ (rivers + named seats).
INSERT INTO regions (id, name, kind) VALUES
    (21, 'Outhold', 'outhold');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 21;
-- +goose StatementEnd
