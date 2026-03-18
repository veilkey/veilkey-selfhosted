CREATE TABLE IF NOT EXISTS function_logs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    function_hash TEXT NOT NULL,
    action        TEXT NOT NULL,
    status        TEXT NOT NULL,
    detail_json   TEXT NOT NULL DEFAULT '{}',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
