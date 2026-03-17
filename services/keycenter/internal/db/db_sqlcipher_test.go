package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestSQLCipherVersionAvailable verifies that the binary is linked against SQLCipher.
func TestSQLCipherVersionAvailable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cipher_version_test.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	version, err := sqlCipherVersion(conn)
	if err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("cipher_version is empty; binary was not built with SQLCipher support")
	}
	t.Logf("SQLCipher version: %s", version)
}

// TestNewFailsClosedWithoutSQLCipherSupport verifies that New() fails
// when VEILKEY_DB_KEY is set but the driver does not support SQLCipher.
// This test is only meaningful when the binary is built WITHOUT SQLCipher.
func TestNewFailsClosedWithoutSQLCipherSupport(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "failclosed_test.db")

	// Check if this environment has SQLCipher support
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	version, _ := sqlCipherVersion(conn)
	_ = conn.Close()
	_ = os.Remove(dbPath)

	if version != "" {
		t.Skip("environment has SQLCipher support; fail-closed test is not applicable")
	}

	_ = os.Setenv("VEILKEY_DB_KEY", "test-key")
	defer func() { _ = os.Unsetenv("VEILKEY_DB_KEY") }()

	_, err = New(dbPath)
	if err == nil {
		t.Fatal("expected New to fail when VEILKEY_DB_KEY is set without SQLCipher support")
	}
}
