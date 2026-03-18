CREATE TABLE IF NOT EXISTS configs (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'LOCAL',
    status     TEXT NOT NULL DEFAULT 'active',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
