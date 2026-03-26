CREATE TABLE IF NOT EXISTS content_version (
    id      INTEGER PRIMARY KEY CHECK (id = 1) DEFAULT 1,
    version INTEGER NOT NULL DEFAULT 0
);

INSERT OR IGNORE INTO content_version (id, version) VALUES (1, 0);
