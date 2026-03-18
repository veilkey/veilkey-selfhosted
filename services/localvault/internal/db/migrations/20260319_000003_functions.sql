CREATE TABLE IF NOT EXISTS functions (
    name           TEXT PRIMARY KEY,
    scope          TEXT NOT NULL,
    vault_hash     TEXT NOT NULL,
    function_hash  TEXT NOT NULL UNIQUE,
    category       TEXT NOT NULL DEFAULT '',
    command        TEXT NOT NULL,
    vars_json      TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    tags_json      TEXT NOT NULL DEFAULT '[]',
    provenance     TEXT NOT NULL DEFAULT 'local',
    last_tested_at DATETIME,
    last_run_at    DATETIME,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
