package main

import (
	"os"
	"path/filepath"
	"testing"
	"veilkey-localvault/internal/db"
)

func TestReadPasswordFromFileEnv(t *testing.T) {
	t.Run("not set returns empty", func(t *testing.T) {
		t.Setenv("VEILKEY_PASSWORD_FILE", "")
		if got := readPasswordFromFileEnv(); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("reads password from file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "pw")
		os.WriteFile(f, []byte("my-secret"), 0600)
		t.Setenv("VEILKEY_PASSWORD_FILE", f)
		if got := readPasswordFromFileEnv(); got != "my-secret" {
			t.Fatalf("got %q, want my-secret", got)
		}
	})

	t.Run("trims trailing newlines", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "pw")
		os.WriteFile(f, []byte("secret\n\r\n"), 0600)
		t.Setenv("VEILKEY_PASSWORD_FILE", f)
		if got := readPasswordFromFileEnv(); got != "secret" {
			t.Fatalf("got %q, want secret", got)
		}
	})
}

func TestDefaultVaultHash(t *testing.T) {
	got := defaultVaultHash("93a8094e-ad3f-4143-b3f3-8551275f24a7")
	if got != "93a8094e" {
		t.Fatalf("defaultVaultHash() = %q", got)
	}
}

func TestRunRebindUpdatesNodeVersion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "localvault.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()
	if err := database.SaveNodeInfo(&db.NodeInfo{
		NodeID:   "node-1",
		DEK:      []byte("01234567890123456789012345678901"),
		DEKNonce: []byte("012345678901"),
		Version:  1,
	}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"veilkey-localvault", "rebind", "--key-version", "9"}
	t.Setenv("VEILKEY_DB_PATH", dbPath)

	runRebind()

	info, err := database.GetNodeInfo()
	if err != nil {
		t.Fatalf("GetNodeInfo: %v", err)
	}
	if info.Version != 9 {
		t.Fatalf("version = %d", info.Version)
	}
}
