-- +goose Up
-- Realm lineage across sealed ages (fate.go): a realm whose name
-- re-forms at the dawn continues its line and counts another age.
-- 1 = a polity of this age; the info panel shows "Nth age" beyond.
ALTER TABLE realms ADD COLUMN age INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE realms DROP COLUMN age;
