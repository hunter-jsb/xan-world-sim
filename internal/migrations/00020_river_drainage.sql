-- +goose Up
-- +goose StatementBegin
-- Drainage = number of upstream rivers (transitively) that feed into
-- this river, including itself. The trunk of a river system has the
-- highest drainage; small headwater stubs have drainage = 1. Lets
-- consumers find the cradle's "Mississippi" by ORDER BY drainage DESC.
ALTER TABLE rivers ADD COLUMN drainage INTEGER NOT NULL DEFAULT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite doesn't support DROP COLUMN cleanly pre-3.35. Migration is
-- one-way; rolling back via "down" is best-effort by recreating the
-- table without the column.
CREATE TABLE rivers_new (
    id   INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
INSERT INTO rivers_new SELECT id, name FROM rivers;
DROP TABLE rivers;
ALTER TABLE rivers_new RENAME TO rivers;
-- +goose StatementEnd
