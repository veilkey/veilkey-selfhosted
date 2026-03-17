package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestConfig_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SaveConfig("DOMAIN", "test.example.com"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	config, err := db.GetConfig("DOMAIN")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Key != "DOMAIN" || config.Value != "test.example.com" {
		t.Errorf("got %s=%s, want DOMAIN=test.example.com", config.Key, config.Value)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Errorf("lifecycle = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestConfig_Upsert(t *testing.T) {
	db := setupTestDB(t)

	db.SaveConfig("PORT", "8080")
	db.SaveConfig("PORT", "9090") // overwrite

	config, _ := db.GetConfig("PORT")
	if config.Value != "9090" {
		t.Errorf("value = %q after upsert, want 9090", config.Value)
	}

	configs, _ := db.ListConfigs()
	if len(configs) != 1 {
		t.Errorf("count = %d after upsert, want 1", len(configs))
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Errorf("lifecycle after upsert = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestConfig_EmptyValueAllowedAtDBLevel(t *testing.T) {
	db := setupTestDB(t)

	// DB allows empty string (NOT NULL constraint passes).
	// API layer blocks it — this tests DB layer only.
	if err := db.SaveConfig("OPTIONAL_FLAG", ""); err != nil {
		t.Fatalf("SaveConfig empty value: %v", err)
	}
	config, err := db.GetConfig("OPTIONAL_FLAG")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Value != "" {
		t.Errorf("value = %q, want empty string", config.Value)
	}
}

func TestConfig_BulkSave(t *testing.T) {
	db := setupTestDB(t)

	err := db.SaveConfigs(map[string]string{
		"DOMAIN":     "test.kr",
		"DB_HOST":    "192.168.1.10",
		"REDIS_HOST": "192.168.1.11",
	})
	if err != nil {
		t.Fatalf("SaveConfigs: %v", err)
	}

	configs, _ := db.ListConfigs()
	if len(configs) != 3 {
		t.Errorf("count = %d, want 3", len(configs))
	}

	count, _ := db.CountConfigs()
	if count != 3 {
		t.Errorf("CountConfigs = %d, want 3", count)
	}
}

func TestConfig_BulkSave_PartialOverwrite(t *testing.T) {
	db := setupTestDB(t)

	db.SaveConfig("EXISTING", "old_value")

	err := db.SaveConfigs(map[string]string{
		"EXISTING": "new_value",
		"NEW_KEY":  "new_val",
	})
	if err != nil {
		t.Fatalf("SaveConfigs: %v", err)
	}

	config, _ := db.GetConfig("EXISTING")
	if config.Value != "new_value" {
		t.Errorf("EXISTING = %q, want new_value", config.Value)
	}

	count, _ := db.CountConfigs()
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Errorf("EXISTING lifecycle = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestConfig_SaveConfigRestoresOperationalLifecycleOnOverwrite(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SaveConfig("APP_URL", "https://old.example.test"); err != nil {
		t.Fatalf("SaveConfig initial: %v", err)
	}
	if err := db.UpdateConfigLifecycle("APP_URL", "EXTERNAL", "block"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}

	if err := db.SaveConfig("APP_URL", "https://new.example.test"); err != nil {
		t.Fatalf("SaveConfig overwrite: %v", err)
	}

	config, err := db.GetConfig("APP_URL")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Value != "https://new.example.test" {
		t.Fatalf("value = %q", config.Value)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Fatalf("lifecycle after overwrite = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestConfig_SaveConfigsRestoresOperationalLifecycleOnOverwrite(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SaveConfig("APP_ENV", "staging"); err != nil {
		t.Fatalf("SaveConfig initial: %v", err)
	}
	if err := db.UpdateConfigLifecycle("APP_ENV", "EXTERNAL", "archive"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}

	if err := db.SaveConfigs(map[string]string{"APP_ENV": "production"}); err != nil {
		t.Fatalf("SaveConfigs overwrite: %v", err)
	}

	config, err := db.GetConfig("APP_ENV")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Value != "production" {
		t.Fatalf("value = %q", config.Value)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Fatalf("lifecycle after bulk overwrite = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestConfig_Delete(t *testing.T) {
	db := setupTestDB(t)

	db.SaveConfig("TO_DELETE", "val")
	if err := db.DeleteConfig("TO_DELETE"); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}
	_, err := db.GetConfig("TO_DELETE")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestConfig_UpdateLifecycle(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SaveConfig("APP_URL", "https://example.test"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := db.UpdateConfigLifecycle("APP_URL", "LOCAL", "active"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}

	config, err := db.GetConfig("APP_URL")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Fatalf("lifecycle = %s/%s", config.Scope, config.Status)
	}

	if err := db.UpdateConfigLifecycle("APP_URL", "", "archive"); err != nil {
		t.Fatalf("UpdateConfigLifecycle archive: %v", err)
	}
	config, err = db.GetConfig("APP_URL")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Scope != "LOCAL" || config.Status != "archive" {
		t.Fatalf("lifecycle after archive = %s/%s", config.Scope, config.Status)
	}
}

func TestConfig_DeleteNotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.DeleteConfig("NONEXISTENT")
	if err == nil {
		t.Error("expected error for deleting nonexistent config")
	}
}

func TestConfig_GetNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetConfig("NONEXISTENT")
	if err == nil {
		t.Error("expected error for nonexistent config")
	}
}

func TestConfig_ListEmpty(t *testing.T) {
	db := setupTestDB(t)

	configs, err := db.ListConfigs()
	if err != nil {
		t.Fatalf("ListConfigs: %v", err)
	}
	if configs != nil {
		t.Errorf("expected nil for empty list, got %d items", len(configs))
	}
}

func TestConfig_SecretsTableUnaffected(t *testing.T) {
	db := setupTestDB(t)

	// Save configs
	db.SaveConfig("DB_PASSWORD", "plaintext_config_value")

	// Save a secret (need node_info first)
	db.SaveNodeInfo(&NodeInfo{
		NodeID:   "test-node",
		DEK:      []byte("0123456789abcdef0123456789abcdef"),
		DEKNonce: []byte("012345678901"),
		Version:  1,
	})
	db.SaveSecret(&Secret{
		ID:         "secret-1",
		Name:       "DB_PASSWORD",
		Ref:        "abcd1234",
		Ciphertext: []byte("encrypted"),
		Nonce:      []byte("nonce"),
		Version:    1,
	})

	// Config and secret with same name coexist independently
	config, _ := db.GetConfig("DB_PASSWORD")
	secret, _ := db.GetSecretByName("DB_PASSWORD")

	if config.Value != "plaintext_config_value" {
		t.Errorf("config value = %q", config.Value)
	}
	if string(secret.Ciphertext) != "encrypted" {
		t.Errorf("secret ciphertext = %q", secret.Ciphertext)
	}

	// Deleting config doesn't affect secret
	db.DeleteConfig("DB_PASSWORD")
	secret2, err := db.GetSecretByName("DB_PASSWORD")
	if err != nil {
		t.Fatalf("secret should still exist after config delete: %v", err)
	}
	if string(secret2.Ciphertext) != "encrypted" {
		t.Error("secret was modified by config delete")
	}
}

func TestMigration_PromotesTempLifecycleToLocalActive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	stmts := []string{
		`CREATE TABLE migrations (version INT PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE secrets (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, ref TEXT, ciphertext BLOB NOT NULL, nonce BLOB NOT NULL, version INT NOT NULL, scope TEXT NOT NULL DEFAULT 'TEMP', status TEXT NOT NULL DEFAULT 'temp', updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE configs (key TEXT PRIMARY KEY, value TEXT NOT NULL, scope TEXT NOT NULL DEFAULT 'TEMP', status TEXT NOT NULL DEFAULT 'temp', updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO migrations(version) VALUES (1),(2),(5),(6),(7),(8)`,
		`INSERT INTO secrets(id,name,ref,ciphertext,nonce,version,scope,status) VALUES ('s1','SECRET_ONE','deadbeef',X'01',X'02',1,'TEMP','temp')`,
		`INSERT INTO configs(key,value,scope,status) VALUES ('APP_URL','https://example.test','TEMP','temp')`,
	}
	for _, stmt := range stmts {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("prep exec %q: %v", stmt, err)
		}
	}

	db, err := New(path)
	if err != nil {
		t.Fatalf("New migrated db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	secret, err := db.GetSecretByName("SECRET_ONE")
	if err != nil {
		t.Fatalf("GetSecretByName: %v", err)
	}
	if secret.Scope != "LOCAL" || secret.Status != "active" {
		t.Fatalf("secret lifecycle = %s/%s, want LOCAL/active", secret.Scope, secret.Status)
	}

	config, err := db.GetConfig("APP_URL")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Fatalf("config lifecycle = %s/%s, want LOCAL/active", config.Scope, config.Status)
	}
}

func TestMigration_AddsOperatorCatalogMetadataColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	stmts := []string{
		`CREATE TABLE migrations (version INT PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE secrets (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, ref TEXT, ciphertext BLOB NOT NULL, nonce BLOB NOT NULL, version INT NOT NULL, scope TEXT NOT NULL DEFAULT 'TEMP', status TEXT NOT NULL DEFAULT 'temp', updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE configs (key TEXT PRIMARY KEY, value TEXT NOT NULL, scope TEXT NOT NULL DEFAULT 'TEMP', status TEXT NOT NULL DEFAULT 'temp', updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE functions (name TEXT PRIMARY KEY, scope TEXT NOT NULL, vault_hash TEXT NOT NULL, function_hash TEXT NOT NULL UNIQUE, category TEXT NOT NULL DEFAULT '', command TEXT NOT NULL, vars_json TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE secret_fields (secret_name TEXT NOT NULL, field_key TEXT NOT NULL, field_type TEXT NOT NULL DEFAULT 'text', ciphertext BLOB NOT NULL, nonce BLOB NOT NULL, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (secret_name, field_key))`,
		`INSERT INTO migrations(version) VALUES (1),(2),(5),(6),(7),(8),(9),(10),(11),(12)`,
	}
	for _, stmt := range stmts {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("prep exec %q: %v", stmt, err)
		}
	}

	db, err := New(path)
	if err != nil {
		t.Fatalf("New migrated db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	checkColumns := func(table string, expected ...string) {
		rows, err := raw.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("PRAGMA table_info(%s): %v", table, err)
		}
		defer rows.Close()

		seen := map[string]bool{}
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			seen[name] = true
		}
		for _, col := range expected {
			if !seen[col] {
				t.Fatalf("expected column %s.%s to exist after migration", table, col)
			}
		}
	}

	checkColumns("secrets", "class", "display_name", "description", "tags_json", "origin", "created_at", "last_rotated_at", "last_revealed_at")
	checkColumns("secret_fields", "field_role", "display_name", "masked_by_default", "required", "sort_order")
	checkColumns("functions", "description", "tags_json", "provenance", "last_tested_at", "last_run_at")
}

func TestSecretMetadataRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	if err := db.SaveNodeInfo(&NodeInfo{
		NodeID:   "test-node",
		DEK:      []byte("0123456789abcdef0123456789abcdef"),
		DEKNonce: []byte("012345678901"),
		Version:  1,
	}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}

	rotatedAt := sql.NullTime{Time: time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second), Valid: true}
	revealedAt := sql.NullTime{Time: time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second), Valid: true}
	secret := &Secret{
		ID:             "secret-meta-1",
		Name:           "OPENAI_API_KEY",
		Ref:            "VK:LOCAL:openai_api_key",
		Ciphertext:     []byte("ciphertext"),
		Nonce:          []byte("nonce"),
		Version:        3,
		Scope:          "LOCAL",
		Status:         "active",
		Class:          "key",
		DisplayName:    "OpenAI API Key",
		Description:    "Primary upstream token",
		TagsJSON:       `["llm","prod"]`,
		Origin:         "localvault",
		LastRotatedAt:  rotatedAt,
		LastRevealedAt: revealedAt,
	}
	if err := db.SaveSecret(secret); err != nil {
		t.Fatalf("SaveSecret: %v", err)
	}

	got, err := db.GetSecretByName(secret.Name)
	if err != nil {
		t.Fatalf("GetSecretByName: %v", err)
	}
	if got.Class != "key" {
		t.Fatalf("Class = %q, want key", got.Class)
	}
	if got.DisplayName != secret.DisplayName {
		t.Fatalf("DisplayName = %q, want %q", got.DisplayName, secret.DisplayName)
	}
	if got.Description != secret.Description {
		t.Fatalf("Description = %q, want %q", got.Description, secret.Description)
	}
	if got.TagsJSON != secret.TagsJSON {
		t.Fatalf("TagsJSON = %q, want %q", got.TagsJSON, secret.TagsJSON)
	}
	if got.Origin != secret.Origin {
		t.Fatalf("Origin = %q, want %q", got.Origin, secret.Origin)
	}
	if !got.LastRotatedAt.Valid || !got.LastRevealedAt.Valid {
		t.Fatalf("expected rotation/reveal timestamps to survive round trip: %#v", got)
	}
}

func TestSecretFieldMetadataRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	fields := []SecretField{
		{
			FieldKey:        "LOGIN_ID",
			FieldType:       "text",
			FieldRole:       "login_id",
			DisplayName:     "Login ID",
			MaskedByDefault: false,
			Required:        true,
			SortOrder:       10,
			Ciphertext:      []byte("encrypted-login"),
			Nonce:           []byte("nonce-login"),
		},
		{
			FieldKey:        "OTP",
			FieldType:       "otp",
			FieldRole:       "otp",
			DisplayName:     "One-time Password",
			MaskedByDefault: true,
			Required:        false,
			SortOrder:       20,
			Ciphertext:      []byte("encrypted-otp"),
			Nonce:           []byte("nonce-otp"),
		},
	}
	if err := db.SaveSecretFields("OPENAI_API_KEY", fields); err != nil {
		t.Fatalf("SaveSecretFields: %v", err)
	}

	list, err := db.ListSecretFields("OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("ListSecretFields: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(ListSecretFields) = %d, want 2", len(list))
	}

	got, err := db.GetSecretField("OPENAI_API_KEY", "OTP")
	if err != nil {
		t.Fatalf("GetSecretField: %v", err)
	}
	if got.FieldRole != "otp" {
		t.Fatalf("FieldRole = %q, want otp", got.FieldRole)
	}
	if got.DisplayName != "One-time Password" {
		t.Fatalf("DisplayName = %q", got.DisplayName)
	}
	if !got.MaskedByDefault {
		t.Fatal("MaskedByDefault = false, want true")
	}
	if got.Required {
		t.Fatal("Required = true, want false")
	}
	if got.SortOrder != 20 {
		t.Fatalf("SortOrder = %d, want 20", got.SortOrder)
	}
}
