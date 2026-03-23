package db

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestDB creates an ephemeral SQLite DB for testing.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	os.Unsetenv("VEILKEY_DB_KEY")
	d, err := New(dbPath)
	if err != nil {
		t.Fatalf("New(%s): %v", dbPath, err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNew_CreateAndPing(t *testing.T) {
	d := newTestDB(t)
	if err := d.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	d := newTestDB(t)
	// Running migrate again should not fail.
	if err := d.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}
