package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitRefusesWhenDBExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a fake existing DB
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatal(err)
	}

	// checkInitDBExists should detect the DB and return an error message
	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error when DB exists without --force, got nil")
	}
	if !contains(err.Error(), "ABORT") {
		t.Errorf("error should contain ABORT, got: %s", err.Error())
	}
}

func TestInitForceOverwritesDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create fake existing DB files
	for _, suffix := range []string{"", "-shm", "-wal"} {
		if err := os.WriteFile(dbPath+suffix, []byte("fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	err := checkInitDBExists(dbPath, true)
	if err != nil {
		t.Fatalf("expected no error with --force, got: %v", err)
	}

	// DB files should be removed
	for _, suffix := range []string{"", "-shm", "-wal"} {
		if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
			t.Errorf("DB file %s should be removed after --force", dbPath+suffix)
		}
	}
}

func TestInitNoDBNoError(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	err := checkInitDBExists(dbPath, false)
	if err != nil {
		t.Fatalf("expected no error when DB does not exist, got: %v", err)
	}
}
