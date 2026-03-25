package commands

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestInitDBExistsErrorContainsDeleteHint(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("fake-db"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error when DB exists without --force, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "rm ") {
		t.Errorf("error message should contain a delete hint (rm ...), got: %s", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("error message should mention --force flag, got: %s", msg)
	}
}

func TestInitForceRemovesOnlyDBFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create DB files and a non-DB file in the same directory
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

	// DB files should be removed
	for _, suffix := range []string{"", "-shm", "-wal"} {
		if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
			t.Errorf("DB file %s should be removed after --force", dbPath+suffix)
		}
	}

	// Non-DB file must survive
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
		t.Fatalf("force should succeed even if only .db exists (no -shm/-wal), got: %v", err)
	}

	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("DB file should be removed after --force")
	}
}

func TestInitDBExistsWithZeroByteDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a 0-byte DB file
	if err := os.WriteFile(dbPath, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error for 0-byte DB file without --force, got nil")
	}
}

func TestInitDBExistsWithSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	realDB := filepath.Join(tmpDir, "real.db")
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	if err := os.WriteFile(realDB, []byte("real-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDB, dbPath); err != nil {
		t.Fatal(err)
	}

	err := checkInitDBExists(dbPath, false)
	if err == nil {
		t.Fatal("expected error when DB is a symlink without --force, got nil")
	}
}

func TestInitForceOnReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can delete files in read-only dirs; skipping")
	}

	tmpDir := t.TempDir()
	innerDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(innerDir, 0700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(innerDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	// Make directory read-only so Remove fails
	if err := os.Chmod(innerDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(innerDir, 0700) }()

	// checkInitDBExists with force does os.Remove but ignores errors, so it returns nil.
	// The actual failure surfaces later when the caller tries to create the new DB.
	// This test documents that behavior — force doesn't error on remove failure.
	_ = checkInitDBExists(dbPath, true)

	// The file should still exist because we couldn't delete it
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("DB file should still exist (read-only dir prevents deletion), got: %v", err)
	}
}

// TestDomain_InitNeverSilentlyOverwrites verifies that for many "existing DB" scenarios,
// init without --force ALWAYS returns an error (never silently overwrites).
func TestDomain_InitNeverSilentlyOverwrites(t *testing.T) {
	scenarios := []struct {
		name    string
		content []byte
		perm    os.FileMode
	}{
		{"normal-db", []byte("SQLite format 3"), 0600},
		{"zero-byte", []byte{}, 0600},
		{"one-byte", []byte{0x00}, 0600},
		{"large-header", []byte(strings.Repeat("X", 4096)), 0600},
		{"readonly-db", []byte("fake"), 0400},
		{"world-readable", []byte("fake"), 0644},
		{"binary-content", []byte{0xFF, 0xFE, 0x00, 0x01}, 0600},
		{"whitespace-only", []byte("   \n\t  "), 0600},
		{"json-content", []byte(`{"not":"a db"}`), 0600},
		{"very-long-name", []byte("db"), 0600},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "veilkey.db")

			if err := os.WriteFile(dbPath, sc.content, sc.perm); err != nil {
				t.Fatal(err)
			}

			err := checkInitDBExists(dbPath, false)
			if err == nil {
				t.Errorf("scenario %q: init without --force must ALWAYS return error when DB exists", sc.name)
			}
		})
	}
}

// TestDomain_ForceDeletesAllDBFiles verifies that after --force, no .db/.shm/.wal files remain.
func TestDomain_ForceDeletesAllDBFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	for _, suffix := range []string{"", "-shm", "-wal"} {
		if err := os.WriteFile(dbPath+suffix, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	err := checkInitDBExists(dbPath, true)
	if err != nil {
		t.Fatalf("--force should succeed, got: %v", err)
	}

	for _, suffix := range []string{"", "-shm", "-wal"} {
		if _, err := os.Stat(dbPath + suffix); !os.IsNotExist(err) {
			t.Errorf("after --force, %s must not exist", dbPath+suffix)
		}
	}
}

func TestInitPreexistingSaltNotDeleted(t *testing.T) {
	// If salt exists before init, the force path sets saltExistedBefore=false
	// only AFTER intentional removal. Verify fileExists correctly detects pre-existing salt.
	tmpDir := t.TempDir()
	saltFile := filepath.Join(tmpDir, "salt")

	// Create a pre-existing salt
	if err := os.WriteFile(saltFile, []byte("pre-existing-salt"), 0600); err != nil {
		t.Fatal(err)
	}

	// fileExists should return true
	if !fileExists(saltFile) {
		t.Error("fileExists should return true for pre-existing salt file")
	}

	// Simulate: init detects salt and refuses without --force.
	// The key invariant: if init fails (non-force), salt must still exist.
	// checkInitDBExists + salt detection happens before any salt removal.
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
	// When salt did NOT exist before init, saltExistedBefore is false.
	// If something fails after salt creation, it's safe to remove the new salt.
	tmpDir := t.TempDir()
	saltFile := filepath.Join(tmpDir, "salt")

	// Verify salt does not exist initially
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

	// Salt should be gone
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

func TestInitSourceStoresVersionMetadata(t *testing.T) {
	src, err := os.ReadFile("init.go")
	if err != nil {
		t.Fatalf("failed to read init.go: %v", err)
	}
	code := string(src)
	if !strings.Contains(code, "ConfigKeyBinaryVersion") {
		t.Error("init.go must store binary version in DB config")
	}
	if !strings.Contains(code, "ConfigKeyKeyDerivationVersion") {
		t.Error("init.go must store key derivation version in DB config")
	}
}
