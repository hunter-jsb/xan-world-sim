-- +goose Up
-- The polity layer: realms (the Crown + independent frontier
-- enclaves), per-seat allegiance, and per-cell territory claims.
INSERT INTO regions (id, name, kind) VALUES (27, 'Crown Capital', 'capital');

CREATE TABLE realms (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    is_crown INTEGER NOT NULL DEFAULT 0,
    seat_x   INTEGER NOT NULL,
    seat_y   INTEGER NOT NULL
);

ALTER TABLE seats ADD COLUMN realm_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE seats ADD COLUMN allegiance REAL NOT NULL DEFAULT 0;

CREATE TABLE territory (
    x        INTEGER NOT NULL,
    y        INTEGER NOT NULL,
    realm_id INTEGER NOT NULL REFERENCES realms(id),
    PRIMARY KEY (x, y)
);

-- +goose Down
DROP TABLE territory;
ALTER TABLE seats DROP COLUMN allegiance;
ALTER TABLE seats DROP COLUMN realm_id;
DROP TABLE realms;
DELETE FROM regions WHERE id = 27;
