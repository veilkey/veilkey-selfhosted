package db

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/veilkey/veilkey-go-package/crypto"
)

// MigrateToEncrypted converts a plaintext SQLite DB to an encrypted SQLCipher DB.
// Called when opening with DB_KEY fails — the DB might be plaintext from an older version.
//
// Returns nil if migration succeeded (caller should retry db.New).
// Returns error if DB is not plaintext or migration failed.
func MigrateToEncrypted(dbPath, dbKey string, kek []byte) error {
	// Verify the KEK through a read-only connection before changing the legacy file.
	path := &url.URL{Scheme: "file", Path: dbPath, RawQuery: "mode=ro"}
	legacy, err := sql.Open("sqlite3", path.String())
	if err != nil {
		return fmt.Errorf("open legacy database: %w", err)
	}
	var dek, nonce []byte
	err = legacy.QueryRow("SELECT dek, dek_nonce FROM node_info LIMIT 1").Scan(&dek, &nonce)
	_ = legacy.Close()
	if err != nil {
		return fmt.Errorf("read legacy node info: %w", err)
	}
	plaintext, err := crypto.Decrypt(kek, dek, nonce)
	if err != nil {
		return fmt.Errorf("verify legacy KEK: %w", err)
	}
	clear(plaintext)
	if _, err := os.Stat(dbPath + ".plaintext.bak"); !os.IsNotExist(err) {
		return fmt.Errorf("plaintext migration backup already exists or cannot be checked")
	}

	// 1. Open plaintext
	dsn := dbPath + "?_journal_mode=wal&_busy_timeout=5000"
	plainDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open plaintext: %w", err)
	}
	defer func() { _ = plainDB.Close() }()

	// 2. Verify readable as plaintext
	if _, err := plainDB.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
		return fmt.Errorf("not a valid plaintext DB: %w", err)
	}

	log.Println("migrate: detected plaintext DB, encrypting...")

	// 3. ATTACH encrypted copy
	encPath := dbPath + ".encrypted"
	if _, err := plainDB.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS encrypted KEY '%s'", encPath, dbKey)); err != nil {
		return fmt.Errorf("attach encrypted: %w", err)
	}

	// 4. Export
	if _, err := plainDB.Exec("SELECT sqlcipher_export('encrypted')"); err != nil {
		_ = os.Remove(encPath)
		return fmt.Errorf("sqlcipher_export: %w", err)
	}
	if _, err := plainDB.Exec("DETACH DATABASE encrypted"); err != nil {
		_ = os.Remove(encPath)
		return fmt.Errorf("detach: %w", err)
	}
	_ = plainDB.Close()

	// 5. Verify encrypted DB is readable with key
	encDSN := encPath + "?_journal_mode=wal&_busy_timeout=5000&_pragma_key=" + dbKey
	verifyDB, err := sql.Open("sqlite3", encDSN)
	if err != nil {
		_ = os.Remove(encPath)
		return fmt.Errorf("verify encrypted open: %w", err)
	}
	if _, err := verifyDB.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
		_ = verifyDB.Close()
		_ = os.Remove(encPath)
		return fmt.Errorf("verify encrypted read: %w", err)
	}
	_ = verifyDB.Close()

	// 6. Atomic rename: plaintext → .bak, encrypted → original
	bakPath := dbPath + ".plaintext.bak"
	if err := os.Rename(dbPath, bakPath); err != nil {
		_ = os.Remove(encPath)
		return fmt.Errorf("backup plaintext: %w", err)
	}
	if err := os.Rename(encPath, dbPath); err != nil {
		_ = os.Rename(bakPath, dbPath)
		return fmt.Errorf("replace with encrypted: %w", err)
	}

	// Keep the backup until the caller has successfully unlocked the encrypted DB.

	log.Println("migrate: plaintext DB encrypted successfully")
	return nil
}
