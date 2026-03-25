package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRefusesWhenDBExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a fake existing DB
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatal(err)
	}

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

func TestInitDBExistsErrorContainsPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error when DB exists without --force, got nil")
	}
	if !strings.Contains(err.Error(), dbPath) {
		t.Errorf("error message should contain the DB path %q, got: %s", dbPath, err.Error())
	}
}

func TestInitForceRemovesOnlyDBFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	for _, suffix := range []string{"", "-shm", "-wal"} {
		if err := os.WriteFile(dbPath+suffix, []byte("fake"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	otherFile := filepath.Join(tmpDir, "salt")
	if err := os.WriteFile(otherFile, []byte("keep-me"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, true)
	if err != nil {
		t.Fatalf("expected no error with --force, got: %v", err)
	}

	for _, suffix := range []string{"", "-shm", "-wal"} {
		if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
			t.Errorf("DB file %s should be removed after --force", dbPath+suffix)
		}
	}

	if _, err := os.Stat(otherFile); err != nil {
		t.Errorf("non-DB file %s should NOT be deleted by --force, got: %v", otherFile, err)
	}
}

func TestInitForceWithMissingShm(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Only create .db — no -shm or -wal
	if err := os.WriteFile(dbPath, []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, true)
	if err != nil {
		t.Fatalf("force should succeed even if only .db exists, got: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("DB file should be removed after --force")
	}
}

func TestInitDBExistsWithZeroByteDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	if err := os.WriteFile(dbPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error for 0-byte DB file without --force, got nil")
	}
}
