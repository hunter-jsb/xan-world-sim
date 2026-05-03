-- +goose Up
-- +goose StatementBegin
-- Dragon pressure on seats — falls off with Chebyshev distance to the
-- nearest dragon den, capped at the implied raid radius from lore.
-- Lore connection: "Northern kingdoms — those nestled up against the
-- Mountain Barrier — live under constant dragon pressure... Larger,
-- calmer settlements form further from the mountains." Higher value
-- = closer to a den = more raid risk.
ALTER TABLE seats ADD COLUMN pressure REAL NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite < 3.35 doesn't support DROP COLUMN; recreate table without it.
CREATE TABLE seats_new (
    x       INTEGER NOT NULL,
    y       INTEGER NOT NULL,
    tier_id INTEGER NOT NULL,
    name    TEXT    NOT NULL,
    PRIMARY KEY (x, y),
    FOREIGN KEY (tier_id) REFERENCES regions(id)
);
INSERT INTO seats_new (x, y, tier_id, name) SELECT x, y, tier_id, name FROM seats;
DROP TABLE seats;
ALTER TABLE seats_new RENAME TO seats;
-- +goose StatementEnd
