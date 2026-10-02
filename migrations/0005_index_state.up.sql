-- One row per vector collection that has been fully backfilled; a new
-- embedder gets a new collection and the worker reindexes it once.
CREATE TABLE index_state (
    collection TEXT PRIMARY KEY,
    reindexed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    docs INT NOT NULL DEFAULT 0
);
