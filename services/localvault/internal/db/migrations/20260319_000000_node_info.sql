CREATE TABLE IF NOT EXISTS node_info (
    node_id    TEXT PRIMARY KEY,
    dek        BLOB NOT NULL,
    dek_nonce  BLOB NOT NULL,
    version    INT DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
