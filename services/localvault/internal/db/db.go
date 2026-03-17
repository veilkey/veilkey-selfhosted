package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

type DB struct {
	conn *sql.DB
}

type NodeInfo struct {
	NodeID   string
	DEK      []byte
	DEKNonce []byte
	Version  int
}

type Secret struct {
	ID             string
	Name           string
	Ref            string
	Ciphertext     []byte
	Nonce          []byte
	Version        int
	Scope          string
	Status         string
	Class          string
	DisplayName    string
	Description    string
	TagsJSON       string
	Origin         string
	CreatedAt      time.Time
	LastRotatedAt  sql.NullTime
	LastRevealedAt sql.NullTime
	UpdatedAt      time.Time
}

type SecretField struct {
	SecretName      string
	FieldKey        string
	FieldType       string
	FieldRole       string
	DisplayName     string
	MaskedByDefault bool
	Required        bool
	SortOrder       int
	Ciphertext      []byte
	Nonce           []byte
	UpdatedAt       time.Time
}

type Config struct {
	Key       string
	Value     string
	Scope     string
	Status    string
	UpdatedAt time.Time
}

type Function struct {
	Name         string       `json:"name"`
	Scope        string       `json:"scope"`
	VaultHash    string       `json:"vault_hash"`
	FunctionHash string       `json:"function_hash"`
	Category     string       `json:"category"`
	Command      string       `json:"command"`
	VarsJSON     string       `json:"vars_json"`
	Description  string       `json:"description"`
	TagsJSON     string       `json:"tags_json"`
	Provenance   string       `json:"provenance"`
	LastTestedAt sql.NullTime `json:"last_tested_at"`
	LastRunAt    sql.NullTime `json:"last_run_at"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type FunctionLog struct {
	ID           int64     `json:"id"`
	FunctionHash string    `json:"function_hash"`
	Action       string    `json:"action"`
	Status       string    `json:"status"`
	DetailJSON   string    `json:"detail_json"`
	CreatedAt    time.Time `json:"created_at"`
}

func New(dbPath string) (*DB, error) {
	dsn := dbPath + "?_journal_mode=wal&_busy_timeout=5000"

	// SQLCipher: 환경변수로 DB 암호화 키가 설정된 경우 DSN에 _pragma_key 추가
	if key := os.Getenv("VEILKEY_DB_KEY"); key != "" {
		dsn += "&_pragma_key=" + url.QueryEscape(key)
	}

	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}

	// SQLCipher 키가 설정된 경우, 드라이버 지원 여부와 DB 접근 가능 여부를 함께 검증
	if os.Getenv("VEILKEY_DB_KEY") != "" {
		version, verErr := sqlCipherVersion(conn)
		if verErr != nil {
			return nil, fmt.Errorf("sqlcipher 지원 확인 실패: %w", verErr)
		}
		if version == "" {
			return nil, fmt.Errorf("VEILKEY_DB_KEY가 설정되었으나 바이너리가 SQLCipher 없이 빌드됨")
		}
		if _, verErr = conn.Exec("SELECT count(*) FROM sqlite_master"); verErr != nil {
			return nil, fmt.Errorf("sqlcipher DB 키 검증 실패: %w", verErr)
		}
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}
	return db, nil
}

// sqlCipherVersion checks if the underlying driver supports SQLCipher.
func sqlCipherVersion(conn *sql.DB) (string, error) {
	var version sql.NullString
	err := conn.QueryRow("PRAGMA cipher_version").Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version.String, nil
}

func (d *DB) migrate() error {
	if err := d.ensureMigrationTable(); err != nil {
		return err
	}
	if err := d.runMigration(1, d.migrateV1); err != nil {
		return err
	}
	if err := d.runMigration(2, d.migrateV2SecretRef); err != nil {
		return err
	}
	if err := d.runMigration(5, d.migrateV5Configs); err != nil {
		return err
	}
	if err := d.runMigration(6, d.migrateV6SecretStatus); err != nil {
		return err
	}
	if err := d.runMigration(7, d.migrateV7ConfigLifecycle); err != nil {
		return err
	}
	if err := d.runMigration(8, d.migrateV8SecretScope); err != nil {
		return err
	}
	if err := d.runMigration(9, d.migrateV9PromoteOperationalRefs); err != nil {
		return err
	}
	if err := d.runMigration(10, d.migrateV10Functions); err != nil {
		return err
	}
	if err := d.runMigration(11, d.migrateV11FunctionLogs); err != nil {
		return err
	}
	if err := d.runMigration(12, d.migrateV12SecretFields); err != nil {
		return err
	}
	if err := d.runMigration(13, d.migrateV13OperatorCatalogMetadata); err != nil {
		return err
	}
	return nil
}

func (d *DB) migrateV1() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS node_info (
			node_id    TEXT PRIMARY KEY,
			dek        BLOB NOT NULL,
			dek_nonce  BLOB NOT NULL,
			version    INT DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			ciphertext BLOB NOT NULL,
			nonce      BLOB NOT NULL,
			version    INT NOT NULL,
			scope      TEXT NOT NULL DEFAULT 'TEMP',
			status     TEXT NOT NULL DEFAULT 'temp',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_secrets_name ON secrets(name)`,
	}
	for _, stmt := range stmts {
		if _, err := d.conn.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) migrateV2SecretRef() error {
	_, err := d.conn.Exec(`ALTER TABLE secrets ADD COLUMN ref TEXT`)
	if err != nil {
		if !isDuplicateColumn(err.Error()) {
			return err
		}
	}
	_, err = d.conn.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_ref ON secrets(ref) WHERE ref IS NOT NULL`)
	return err
}

func isDuplicateColumn(msg string) bool {
	return len(msg) >= 9 && msg[:9] == "duplicate"
}

func (d *DB) ensureMigrationTable() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		version INT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (d *DB) runMigration(version int, fn func() error) error {
	var count int
	d.conn.QueryRow(`SELECT COUNT(*) FROM migrations WHERE version = ?`, version).Scan(&count)
	if count > 0 {
		return nil
	}
	if err := fn(); err != nil {
		return fmt.Errorf("migration v%d failed: %w", version, err)
	}
	_, err := d.conn.Exec(`INSERT INTO migrations (version) VALUES (?)`, version)
	return err
}

func (d *DB) migrateV5Configs() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS configs (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		scope      TEXT NOT NULL DEFAULT 'TEMP',
		status     TEXT NOT NULL DEFAULT 'temp',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (d *DB) migrateV6SecretStatus() error {
	_, err := d.conn.Exec(`ALTER TABLE secrets ADD COLUMN status TEXT NOT NULL DEFAULT 'temp'`)
	if err != nil {
		if !isDuplicateColumn(err.Error()) {
			return err
		}
	}
	_, err = d.conn.Exec(`UPDATE secrets SET status = 'temp' WHERE status IS NULL OR status = ''`)
	return err
}

func (d *DB) migrateV7ConfigLifecycle() error {
	_, err := d.conn.Exec(`ALTER TABLE configs ADD COLUMN scope TEXT NOT NULL DEFAULT 'TEMP'`)
	if err != nil && !isDuplicateColumn(err.Error()) {
		return err
	}
	_, err = d.conn.Exec(`ALTER TABLE configs ADD COLUMN status TEXT NOT NULL DEFAULT 'temp'`)
	if err != nil && !isDuplicateColumn(err.Error()) {
		return err
	}
	if _, err := d.conn.Exec(`UPDATE configs SET scope = 'TEMP' WHERE scope IS NULL OR scope = ''`); err != nil {
		return err
	}
	_, err = d.conn.Exec(`UPDATE configs SET status = 'temp' WHERE status IS NULL OR status = ''`)
	return err
}

func (d *DB) migrateV8SecretScope() error {
	_, err := d.conn.Exec(`ALTER TABLE secrets ADD COLUMN scope TEXT NOT NULL DEFAULT 'TEMP'`)
	if err != nil && !isDuplicateColumn(err.Error()) {
		return err
	}
	_, err = d.conn.Exec(`UPDATE secrets SET scope = 'TEMP' WHERE scope IS NULL OR scope = ''`)
	return err
}

func (d *DB) migrateV9PromoteOperationalRefs() error {
	stmts := []string{
		`UPDATE secrets SET scope = 'LOCAL', status = 'active' WHERE (scope IS NULL OR scope = '' OR scope = 'TEMP') AND (status IS NULL OR status = '' OR status = 'temp')`,
		`UPDATE configs SET scope = 'LOCAL', status = 'active' WHERE (scope IS NULL OR scope = '' OR scope = 'TEMP') AND (status IS NULL OR status = '' OR status = 'temp')`,
	}
	for _, stmt := range stmts {
		if _, err := d.conn.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) migrateV10Functions() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS functions (
		name          TEXT PRIMARY KEY,
		scope         TEXT NOT NULL,
		vault_hash    TEXT NOT NULL,
		function_hash TEXT NOT NULL UNIQUE,
		category      TEXT NOT NULL DEFAULT '',
		command       TEXT NOT NULL,
		vars_json     TEXT NOT NULL,
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (d *DB) migrateV11FunctionLogs() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS function_logs (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		function_hash TEXT NOT NULL,
		action        TEXT NOT NULL,
		status        TEXT NOT NULL,
		detail_json   TEXT NOT NULL DEFAULT '{}',
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (d *DB) migrateV12SecretFields() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS secret_fields (
		secret_name TEXT NOT NULL,
		field_key   TEXT NOT NULL,
		field_type  TEXT NOT NULL DEFAULT 'text',
		ciphertext  BLOB NOT NULL,
		nonce       BLOB NOT NULL,
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (secret_name, field_key),
		FOREIGN KEY (secret_name) REFERENCES secrets(name) ON DELETE CASCADE
	)`)
	return err
}

func (d *DB) migrateV13OperatorCatalogMetadata() error {
	stmts := []string{
		`ALTER TABLE secrets ADD COLUMN class TEXT NOT NULL DEFAULT 'key'`,
		`ALTER TABLE secrets ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secrets ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secrets ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE secrets ADD COLUMN origin TEXT NOT NULL DEFAULT 'sync'`,
		`ALTER TABLE secrets ADD COLUMN created_at DATETIME`,
		`ALTER TABLE secrets ADD COLUMN last_rotated_at DATETIME`,
		`ALTER TABLE secrets ADD COLUMN last_revealed_at DATETIME`,
		`ALTER TABLE secret_fields ADD COLUMN field_role TEXT NOT NULL DEFAULT 'text'`,
		`ALTER TABLE secret_fields ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE secret_fields ADD COLUMN masked_by_default INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE secret_fields ADD COLUMN required INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE secret_fields ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE functions ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE functions ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE functions ADD COLUMN provenance TEXT NOT NULL DEFAULT 'local'`,
		`ALTER TABLE functions ADD COLUMN last_tested_at DATETIME`,
		`ALTER TABLE functions ADD COLUMN last_run_at DATETIME`,
	}
	for _, stmt := range stmts {
		if _, err := d.conn.Exec(stmt); err != nil && !isDuplicateColumn(err.Error()) {
			return err
		}
	}
	if _, err := d.conn.Exec(`UPDATE secrets SET created_at = updated_at WHERE created_at IS NULL`); err != nil {
		return err
	}
	return nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Ping() error {
	return d.conn.Ping()
}

// --- Node Info ---

func (d *DB) GetNodeInfo() (*NodeInfo, error) {
	var info NodeInfo
	err := d.conn.QueryRow(`SELECT node_id, dek, dek_nonce, version FROM node_info LIMIT 1`).
		Scan(&info.NodeID, &info.DEK, &info.DEKNonce, &info.Version)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *DB) SaveNodeInfo(info *NodeInfo) error {
	_, err := d.conn.Exec(`INSERT INTO node_info (node_id, dek, dek_nonce, version) VALUES (?, ?, ?, ?)`,
		info.NodeID, info.DEK, info.DEKNonce, info.Version)
	return err
}

func (d *DB) UpdateNodeDEK(dek, nonce []byte, version int) error {
	result, err := d.conn.Exec(`UPDATE node_info SET dek = ?, dek_nonce = ?, version = ?`, dek, nonce, version)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no node_info to update")
	}
	return nil
}

func (d *DB) UpdateNodeVersion(version int) error {
	result, err := d.conn.Exec(`UPDATE node_info SET version = ?`, version)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no node_info to update")
	}
	return nil
}

// --- Secrets ---

func (d *DB) SaveSecret(secret *Secret) error {
	if secret.Status == "" {
		secret.Status = "active"
	}
	if secret.Scope == "" {
		secret.Scope = "LOCAL"
	}
	_, err := d.conn.Exec(`
		INSERT OR REPLACE INTO secrets (
			id, name, ref, ciphertext, nonce, version, scope, status,
			class, display_name, description, tags_json, origin,
			created_at, last_rotated_at, last_revealed_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?,
			COALESCE(NULLIF(?, ''), 'key'),
			COALESCE(NULLIF(?, ''), ?),
			COALESCE(?, ''),
			COALESCE(NULLIF(?, ''), '[]'),
			COALESCE(NULLIF(?, ''), 'sync'),
			COALESCE(?, (SELECT created_at FROM secrets WHERE name = ?), CURRENT_TIMESTAMP),
			?, ?, CURRENT_TIMESTAMP
		)`,
		secret.ID, secret.Name, secret.Ref, secret.Ciphertext, secret.Nonce, secret.Version, secret.Scope, secret.Status,
		secret.Class, secret.DisplayName, secret.Name, secret.Description, secret.TagsJSON, secret.Origin,
		nullTimeValue(secret.CreatedAt), secret.Name, nullTimeValue(secret.LastRotatedAt), nullTimeValue(secret.LastRevealedAt))
	return err
}

func (d *DB) GetSecretByName(name string) (*Secret, error) {
	var s Secret
	var ref sql.NullString
	err := d.conn.QueryRow(`
		SELECT id, name, ref, ciphertext, nonce, version, scope, status,
		       class, display_name, description, tags_json, origin,
		       created_at, last_rotated_at, last_revealed_at, updated_at
		FROM secrets WHERE name = ?`, name).
		Scan(&s.ID, &s.Name, &ref, &s.Ciphertext, &s.Nonce, &s.Version, &s.Scope, &s.Status,
			&s.Class, &s.DisplayName, &s.Description, &s.TagsJSON, &s.Origin,
			&s.CreatedAt, &s.LastRotatedAt, &s.LastRevealedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("secret %s not found", name)
	}
	if ref.Valid {
		s.Ref = ref.String
	}
	return &s, nil
}

func (d *DB) GetSecretByRef(refHash string) (*Secret, error) {
	var s Secret
	var ref sql.NullString
	err := d.conn.QueryRow(`
		SELECT id, name, ref, ciphertext, nonce, version, scope, status,
		       class, display_name, description, tags_json, origin,
		       created_at, last_rotated_at, last_revealed_at, updated_at
		FROM secrets WHERE ref = ?`, refHash).
		Scan(&s.ID, &s.Name, &ref, &s.Ciphertext, &s.Nonce, &s.Version, &s.Scope, &s.Status,
			&s.Class, &s.DisplayName, &s.Description, &s.TagsJSON, &s.Origin,
			&s.CreatedAt, &s.LastRotatedAt, &s.LastRevealedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("secret ref %s not found", refHash)
	}
	if ref.Valid {
		s.Ref = ref.String
	}
	return &s, nil
}

func (d *DB) ListSecrets() ([]Secret, error) {
	rows, err := d.conn.Query(`
		SELECT id, name, ref, ciphertext, nonce, version, scope, status,
		       class, display_name, description, tags_json, origin,
		       created_at, last_rotated_at, last_revealed_at, updated_at
		FROM secrets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []Secret
	for rows.Next() {
		var s Secret
		var ref sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &ref, &s.Ciphertext, &s.Nonce, &s.Version, &s.Scope, &s.Status,
			&s.Class, &s.DisplayName, &s.Description, &s.TagsJSON, &s.Origin,
			&s.CreatedAt, &s.LastRotatedAt, &s.LastRevealedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if ref.Valid {
			s.Ref = ref.String
		}
		secrets = append(secrets, s)
	}
	return secrets, nil
}

func (d *DB) DeleteSecret(name string) error {
	result, err := d.conn.Exec(`DELETE FROM secrets WHERE name = ?`, name)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret %s not found", name)
	}
	return nil
}

func (d *DB) UpdateSecretStatus(refHash, status string) error {
	result, err := d.conn.Exec(`UPDATE secrets SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE ref = ?`, status, refHash)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) UpdateSecretLifecycle(refHash, scope, status string) error {
	result, err := d.conn.Exec(`UPDATE secrets SET scope = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE ref = ?`, scope, status, refHash)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) MarkSecretRevealed(refHash string, revealedAt time.Time) error {
	result, err := d.conn.Exec(`UPDATE secrets SET last_revealed_at = ?, updated_at = CURRENT_TIMESTAMP WHERE ref = ?`, revealedAt.UTC(), refHash)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) MarkSecretRotated(refHash string, rotatedAt time.Time) error {
	result, err := d.conn.Exec(`UPDATE secrets SET last_rotated_at = ?, updated_at = CURRENT_TIMESTAMP WHERE ref = ?`, rotatedAt.UTC(), refHash)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret ref %s not found", refHash)
	}
	return nil
}

func (d *DB) CountSecrets() (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&count)
	return count, err
}

func (d *DB) SaveSecretFields(secretName string, fields []SecretField) error {
	if secretName == "" {
		return fmt.Errorf("secret name is required")
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO secret_fields (
			secret_name, field_key, field_type, field_role, display_name,
			masked_by_default, required, sort_order, ciphertext, nonce, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(secret_name, field_key) DO UPDATE SET
			field_type = excluded.field_type,
			field_role = excluded.field_role,
			display_name = excluded.display_name,
			masked_by_default = excluded.masked_by_default,
			required = excluded.required,
			sort_order = excluded.sort_order,
			ciphertext = excluded.ciphertext,
			nonce = excluded.nonce,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, field := range fields {
		role := field.FieldRole
		if role == "" {
			role = field.FieldType
		}
		if _, err := stmt.Exec(
			secretName,
			field.FieldKey,
			field.FieldType,
			role,
			coalesceString(field.DisplayName, field.FieldKey),
			boolToInt(field.MaskedByDefault),
			boolToInt(field.Required),
			field.SortOrder,
			field.Ciphertext,
			field.Nonce,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) ListSecretFields(secretName string) ([]SecretField, error) {
	rows, err := d.conn.Query(`
		SELECT secret_name, field_key, field_type, field_role, display_name,
		       masked_by_default, required, sort_order, ciphertext, nonce, updated_at
		FROM secret_fields
		WHERE secret_name = ?
		ORDER BY field_key
	`, secretName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []SecretField
	for rows.Next() {
		var field SecretField
		var masked, required int
		if err := rows.Scan(&field.SecretName, &field.FieldKey, &field.FieldType, &field.FieldRole, &field.DisplayName,
			&masked, &required, &field.SortOrder, &field.Ciphertext, &field.Nonce, &field.UpdatedAt); err != nil {
			return nil, err
		}
		field.MaskedByDefault = masked != 0
		field.Required = required != 0
		fields = append(fields, field)
	}
	return fields, nil
}

func (d *DB) GetSecretField(secretName, fieldKey string) (*SecretField, error) {
	var field SecretField
	var masked, required int
	err := d.conn.QueryRow(`
		SELECT secret_name, field_key, field_type, field_role, display_name,
		       masked_by_default, required, sort_order, ciphertext, nonce, updated_at
		FROM secret_fields
		WHERE secret_name = ? AND field_key = ?
	`, secretName, fieldKey).Scan(&field.SecretName, &field.FieldKey, &field.FieldType, &field.FieldRole, &field.DisplayName,
		&masked, &required, &field.SortOrder, &field.Ciphertext, &field.Nonce, &field.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("secret field %s.%s not found", secretName, fieldKey)
	}
	field.MaskedByDefault = masked != 0
	field.Required = required != 0
	return &field, nil
}

func (d *DB) DeleteSecretField(secretName, fieldKey string) error {
	result, err := d.conn.Exec(`DELETE FROM secret_fields WHERE secret_name = ? AND field_key = ?`, secretName, fieldKey)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret field %s.%s not found", secretName, fieldKey)
	}
	return nil
}

func coalesceString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func nullTimeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return nil
		}
		return v
	case sql.NullTime:
		if !v.Valid {
			return nil
		}
		return v.Time
	default:
		return nil
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// --- Configs (plaintext key-value) ---

func (d *DB) SaveConfig(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO configs (key, value, scope, status, updated_at)
		VALUES (?, ?, 'LOCAL', 'active', CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			scope = 'LOCAL',
			status = 'active',
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (d *DB) SaveConfigs(configs map[string]string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO configs (key, value, scope, status, updated_at)
		VALUES (?, ?, 'LOCAL', 'active', CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			scope = 'LOCAL',
			status = 'active',
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for k, v := range configs {
		if _, err := stmt.Exec(k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) GetConfig(key string) (*Config, error) {
	var c Config
	err := d.conn.QueryRow(`SELECT key, value, scope, status, updated_at FROM configs WHERE key = ?`, key).
		Scan(&c.Key, &c.Value, &c.Scope, &c.Status, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("config %s not found", key)
	}
	return &c, nil
}

func (d *DB) ListConfigs() ([]Config, error) {
	rows, err := d.conn.Query(`SELECT key, value, scope, status, updated_at FROM configs ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []Config
	for rows.Next() {
		var c Config
		if err := rows.Scan(&c.Key, &c.Value, &c.Scope, &c.Status, &c.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, nil
}

func (d *DB) DeleteConfig(key string) error {
	result, err := d.conn.Exec(`DELETE FROM configs WHERE key = ?`, key)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("config %s not found", key)
	}
	return nil
}

func (d *DB) CountConfigs() (int, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(*) FROM configs`).Scan(&count)
	return count, err
}

func (d *DB) UpdateConfigLifecycle(key, scope, status string) error {
	if scope == "" {
		_, err := d.conn.Exec(`UPDATE configs SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`, status, key)
		if err != nil {
			return err
		}
	} else {
		_, err := d.conn.Exec(`UPDATE configs SET scope = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`, scope, status, key)
		if err != nil {
			return err
		}
	}
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM configs WHERE key = ?`, key).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("config %s not found", key)
	}
	return nil
}

func (d *DB) ReencryptAllSecrets(
	decryptFn func(ciphertext, nonce []byte) ([]byte, error),
	encryptFn func(plaintext []byte) (ciphertext, nonce []byte, err error),
	newVersion int,
) (int, error) {
	secrets, err := d.ListSecrets()
	if err != nil {
		return 0, err
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0
	for _, s := range secrets {
		plaintext, err := decryptFn(s.Ciphertext, s.Nonce)
		if err != nil {
			return count, fmt.Errorf("decrypt secret %s: %w", s.Name, err)
		}
		newCiphertext, newNonce, err := encryptFn(plaintext)
		if err != nil {
			return count, fmt.Errorf("encrypt secret %s: %w", s.Name, err)
		}
		_, err = tx.Exec(`UPDATE secrets SET ciphertext = ?, nonce = ?, version = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newCiphertext, newNonce, newVersion, s.ID)
		if err != nil {
			return count, err
		}
		if _, err := tx.Exec(`UPDATE secrets SET last_rotated_at = CURRENT_TIMESTAMP WHERE id = ?`, s.ID); err != nil {
			return count, err
		}
		count++
	}

	return count, tx.Commit()
}

func (d *DB) ReencryptMixedSecrets(
	decryptOldFn func(ciphertext, nonce []byte) ([]byte, error),
	decryptCurrentFn func(ciphertext, nonce []byte) ([]byte, error),
	encryptFn func(plaintext []byte) (ciphertext, nonce []byte, err error),
	newVersion int,
) (int, int, error) {
	secrets, err := d.ListSecrets()
	if err != nil {
		return 0, 0, err
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	updated := 0
	skipped := 0
	for _, s := range secrets {
		if s.Version == newVersion && decryptCurrentFn != nil {
			if _, err := decryptCurrentFn(s.Ciphertext, s.Nonce); err == nil {
				skipped++
				continue
			}
		}

		plaintext, err := decryptOldFn(s.Ciphertext, s.Nonce)
		if err != nil {
			if decryptCurrentFn != nil {
				if _, currentErr := decryptCurrentFn(s.Ciphertext, s.Nonce); currentErr == nil {
					skipped++
					continue
				}
			}
			return updated, skipped, fmt.Errorf("decrypt secret %s: %w", s.Name, err)
		}

		newCiphertext, newNonce, err := encryptFn(plaintext)
		if err != nil {
			return updated, skipped, fmt.Errorf("encrypt secret %s: %w", s.Name, err)
		}
		_, err = tx.Exec(`UPDATE secrets SET ciphertext = ?, nonce = ?, version = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			newCiphertext, newNonce, newVersion, s.ID)
		if err != nil {
			return updated, skipped, err
		}
		if _, err := tx.Exec(`UPDATE secrets SET last_rotated_at = CURRENT_TIMESTAMP WHERE id = ?`, s.ID); err != nil {
			return updated, skipped, err
		}
		updated++
	}

	return updated, skipped, tx.Commit()
}
