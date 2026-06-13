-- +goose Up
-- Fate: the bridge between the engine's two motions. A sealed slice
-- distills into a fate record — the next deep-time step folds it in.
--
-- tells: ancient ruins on a generated (fated) world — fate ruins that
-- survived reconciliation against the new era's geography. Rendered
-- with the existing 'ruin' kind; story carries the old chronicle line.
-- Rewritten by Persist like every world table.
CREATE TABLE tells (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    x INTEGER NOT NULL,
    y INTEGER NOT NULL,
    story TEXT NOT NULL,
    era_kya INTEGER NOT NULL
);

-- fates: the chain itself — one JSON record per sealed age, keyed by
-- (seed, kya). Cross-generation state: survives regens and scrubbing,
-- per seed. Sealing at kya K rewrites the future: rows with kya <= K
-- for that seed are dropped first (branch semantics).
CREATE TABLE fates (
    seed INTEGER NOT NULL,
    kya INTEGER NOT NULL,
    record TEXT NOT NULL,
    PRIMARY KEY (seed, kya)
);

-- +goose Down
DROP TABLE fates;
DROP TABLE tells;
