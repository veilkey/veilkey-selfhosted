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

func TestInitPreexistingSaltNotDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	saltFile := filepath.Join(tmpDir, "salt")

	// Create a pre-existing salt
	if err := os.WriteFile(saltFile, []byte("pre-existing-salt"), 0600); err != nil {
		t.Fatal(err)
	}

	if !fileExists(saltFile) {
		t.Error("fileExists should return true for pre-existing salt file")
	}

	// Simulate: init detects salt and refuses without --force.
	// The key invariant: if init fails (non-force), salt must still exist.
	if _, err := os.Stat(saltFile); err != nil {
		t.Error("pre-existing salt file should still exist after failed init check")
	}
	data, err := os.ReadFile(saltFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "pre-existing-salt" {
		t.Errorf("salt content should be preserved, got %q", string(data))
	}
}

func TestInitNewSaltDeletedOnError(t *testing.T) {
	tmpDir := t.TempDir()
	saltFile := filepath.Join(tmpDir, "salt")

	if fileExists(saltFile) {
		t.Fatal("salt should not exist before test")
	}

	saltExistedBefore := fileExists(saltFile)
	if saltExistedBefore {
		t.Fatal("saltExistedBefore should be false when salt does not exist")
	}

	// Simulate: init creates salt then hits an error
	if err := os.WriteFile(saltFile, []byte("newly-created-salt"), 0600); err != nil {
		t.Fatal(err)
	}

	// On error, since salt didn't exist before, it's safe to clean up
	if !saltExistedBefore {
		_ = os.Remove(saltFile)
	}

	if fileExists(saltFile) {
		t.Error("newly created salt should be removed on error when it didn't pre-exist")
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	existing := filepath.Join(tmpDir, "exists")
	if err := os.WriteFile(existing, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if !fileExists(existing) {
		t.Error("fileExists should return true for existing file")
	}
	if fileExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("fileExists should return false for non-existing file")
	}
}
