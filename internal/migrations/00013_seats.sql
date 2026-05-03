-- +goose Up
-- +goose StatementBegin
-- Salmon lord seats — settlements on major rivers in cradle/foothill
-- terrain. The lord typology in the lore (Tributary, March, Reach,
-- Headwater hold, Outhold) maps onto specific procgen criteria;
-- this first cut lands the salmon-lord (Tributary) tier as a single
-- region kind. Future iterations can split by type.
INSERT INTO regions (id, name, kind) VALUES
    (18, 'Salmon Lord Seat', 'seat');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id = 18;
-- +goose StatementEnd
