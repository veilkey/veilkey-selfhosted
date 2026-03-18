CREATE TABLE IF NOT EXISTS secret_fields (
    secret_name       TEXT NOT NULL,
    field_key         TEXT NOT NULL,
    field_type        TEXT NOT NULL DEFAULT 'text',
    field_role        TEXT NOT NULL DEFAULT 'text',
    display_name      TEXT NOT NULL DEFAULT '',
    masked_by_default INTEGER NOT NULL DEFAULT 1,
    required          INTEGER NOT NULL DEFAULT 0,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    ciphertext        BLOB NOT NULL,
    nonce             BLOB NOT NULL,
    updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (secret_name, field_key),
    FOREIGN KEY (secret_name) REFERENCES secrets(name) ON DELETE CASCADE
);
