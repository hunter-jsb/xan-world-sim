-- +goose Up
-- +goose StatementBegin
-- Extend the lord typology beyond the salmon-lord (Tributary) tier.
-- Each new tier maps to a distinct geographic signature so the seed
-- alone determines who's where:
--   March        — foothill/cradle directly adjacent to a mountain
--                  massif. "We are the wall." Defense against the
--                  mountain wilds is the seat's reason for being.
--   Headwater    — river head (Ord=1) of a major river. Sacred-source
--                  geography, contested by religious orders, closest
--                  to dwarven territory in the lore.
-- (Reach, Outhold reserved for later — they need richer signals
-- like distance-from-heartland or off-network detection.)
INSERT INTO regions (id, name, kind) VALUES
    (19, 'March Seat', 'march'),
    (20, 'Headwater Hold', 'headwater');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM regions WHERE id IN (19, 20);
-- +goose StatementEnd
