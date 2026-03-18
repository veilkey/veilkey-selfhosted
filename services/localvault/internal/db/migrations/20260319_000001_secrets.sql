CREATE TABLE IF NOT EXISTS secrets (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,
    ref              TEXT,
    ciphertext       BLOB NOT NULL,
    nonce            BLOB NOT NULL,
    version          INT NOT NULL,
    scope            TEXT NOT NULL DEFAULT 'LOCAL',
    status           TEXT NOT NULL DEFAULT 'active',
    class            TEXT NOT NULL DEFAULT 'key',
    display_name     TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    tags_json        TEXT NOT NULL DEFAULT '[]',
    origin           TEXT NOT NULL DEFAULT 'sync',
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_rotated_at  DATETIME,
    last_revealed_at DATETIME,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_secrets_name ON secrets(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_ref ON secrets(ref) WHERE ref IS NOT NULL;
