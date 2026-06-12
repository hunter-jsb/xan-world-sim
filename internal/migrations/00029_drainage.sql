-- +goose Up
-- Per-cell flow accumulation (upstream land cells), stamped from the
-- generator's D8 pass over pit-filled bedrock. Geology, not climate:
-- identical across kya for a seed. The hydrology lens reads it.
ALTER TABLE region_cells ADD COLUMN drainage INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE region_cells DROP COLUMN drainage;
