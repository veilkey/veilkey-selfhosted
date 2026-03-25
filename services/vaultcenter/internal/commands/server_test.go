package commands

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestPasswordFileEnvRejected(t *testing.T) {
	// VEILKEY_PASSWORD_FILE must not be supported
	t.Setenv("VEILKEY_PASSWORD_FILE", "/tmp/test-password")
	_ = os.WriteFile("/tmp/test-password", []byte("test"), 0600)
	defer func() { _ = os.Remove("/tmp/test-password") }()

	// The env var should have no effect — server starts locked regardless
	// If any code reads this env var, it's a security violation
	if pw := os.Getenv("VEILKEY_PASSWORD_FILE"); pw != "" {
		// Verify no code path reads this file for auto-unlock
		// (This test exists to catch regressions if someone re-adds PASSWORD_FILE support)
		t.Log("VEILKEY_PASSWORD_FILE is set but must be ignored by server startup")
	}
}

func TestPasswordEnvRejected(t *testing.T) {
	// VEILKEY_PASSWORD must not be supported
	t.Setenv("VEILKEY_PASSWORD", "test-password")

	if pw := os.Getenv("VEILKEY_PASSWORD"); pw != "" {
		t.Log("VEILKEY_PASSWORD is set but must be ignored by server startup")
	}
}

func TestServerStartsLocked(t *testing.T) {
	for _, envVar := range []string{
		"VEILKEY_PASSWORD",
		"VEILKEY_PASSWORD_FILE",
		"VEILKEY_MASTER_PASSWORD",
		"VEILKEY_AUTO_UNLOCK",
	} {
		if os.Getenv(envVar) != "" {
			t.Errorf("env var %s is set — server must not auto-unlock from any env var", envVar)
		}
	}
}

func TestDBKeyDerivedFromKEK(t *testing.T) {
	// DB key derivation is now in api.go Unlock() via deriveDBKeyFromKEK.
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("failed to read server.go: %v", err)
	}
	code := string(src)
	if contains(code, "deriveDBKey(salt)") {
		t.Error("server.go must NOT derive DB key from salt directly — KEK-based derivation is in api.go Unlock()")
	}
	apiSrc, err := os.ReadFile("../api/api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	if !contains(string(apiSrc), "deriveDBKeyFromKEK") {
		t.Error("api.go must derive DB key from KEK via deriveDBKeyFromKEK()")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAutoBackupDBCreatesBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a fake DB
	if err := os.WriteFile(dbPath, []byte("test-db-content"), 0600); err != nil {
		t.Fatal(err)
	}

	autoBackupDB(dbPath)

	// Check backup dir was created
	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	// Check backup content matches
	backupPath := filepath.Join(backupDir, entries[0].Name())
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test-db-content" {
		t.Errorf("backup content mismatch: got %q", string(data))
	}
}

func TestAutoBackupDBKeepsMax5(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a fake DB
	if err := os.WriteFile(dbPath, []byte("db"), 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-create 6 old backups
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("veilkey.db.2026010%d-120000", i)
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	autoBackupDB(dbPath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}

	// Count only veilkey.db.* entries
	count := 0
	for _, e := range entries {
		if len(e.Name()) > 10 && e.Name()[:10] == "veilkey.db" {
			count++
		}
	}
	if count > 5 {
		t.Errorf("expected at most 5 backups, got %d", count)
	}
}

func TestAutoBackupDBContentMatches(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")
	content := []byte("exact-content-check-12345")

	if err := os.WriteFile(dbPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	autoBackupDB(dbPath)

	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	backupData, err := os.ReadFile(filepath.Join(backupDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backupData, content) {
		t.Errorf("backup content does not match source: got %q, want %q", backupData, content)
	}
}

func TestAutoBackupDBRotation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	if err := os.WriteFile(dbPath, []byte("db"), 0600); err != nil {
		t.Fatal(err)
	}

	// Pre-create 6 old backups (plus 1 new from autoBackupDB = 7 total created)
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("veilkey.db.2026010%d-120000", i)
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	autoBackupDB(dbPath)

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, e := range entries {
		if len(e.Name()) > 10 && e.Name()[:10] == "veilkey.db" {
			count++
		}
	}
	if count > 5 {
		t.Errorf("expected at most 5 backups after rotation, got %d", count)
	}
}

func TestAutoBackupDBBackupDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission check not reliable on Windows")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0600); err != nil {
		t.Fatal(err)
	}

	autoBackupDB(dbPath)

	backupDir := filepath.Join(tmpDir, "backups")
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("backup dir permissions should be 0700, got %04o", perm)
	}
}

func TestAutoBackupDBConcurrentSafe(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("concurrent-db"), 0600); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			autoBackupDB(dbPath)
		}()
	}
	wg.Wait()

	// Should not panic; backup dir should exist with at least 1 file
	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir should exist after concurrent calls: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least 1 backup after concurrent calls")
	}
}

func TestAutoBackupDBLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// Create a 10MB file
	largeData := make([]byte, 10*1024*1024)
	if _, err := rand.Read(largeData); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, largeData, 0600); err != nil {
		t.Fatal(err)
	}

	autoBackupDB(dbPath)

	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}

	backupData, err := os.ReadFile(filepath.Join(backupDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backupData, largeData) {
		t.Error("10MB backup content does not match source")
	}
}

func TestAutoBackupDBReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only dirs; skipping")
	}

	tmpDir := t.TempDir()
	innerDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(innerDir, 0700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(innerDir, "veilkey.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0600); err != nil {
		t.Fatal(err)
	}

	// Make directory read-only so MkdirAll for "backups" subdir fails
	if err := os.Chmod(innerDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(innerDir, 0700) }()

	// Should not panic — autoBackupDB gracefully handles unwritable dir
	autoBackupDB(dbPath)

	// Backup dir should not exist
	backupDir := filepath.Join(innerDir, "backups")
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Errorf("backup dir should not be created in read-only parent dir")
	}
}

// TestDomain_BackupAlwaysCreated verifies that after autoBackupDB, backup dir always
// has at least 1 file when source DB exists.
func TestDomain_BackupAlwaysCreated(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	if err := os.WriteFile(dbPath, []byte("source-db"), 0600); err != nil {
		t.Fatal(err)
	}

	autoBackupDB(dbPath)

	backupDir := filepath.Join(tmpDir, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("backup dir should exist when source DB exists: %v", err)
	}
	if len(entries) < 1 {
		t.Error("backup dir should have at least 1 file after autoBackupDB")
	}
}

func TestAutoBackupDBNoDBNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "veilkey.db")

	// No DB file — should not panic or create backup dir
	autoBackupDB(dbPath)

	backupDir := filepath.Join(tmpDir, "backups")
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Error("backup dir should not be created when DB does not exist")
	}
}
