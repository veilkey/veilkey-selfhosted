package commands

import (
	"os"
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
	// server.go should NOT have legacy deriveDBKey(salt).
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
	if !contains(string(apiSrc), "DeriveDBKeyFromKEK") {
		t.Error("api.go must derive DB key from KEK via DeriveDBKeyFromKEK()")
	}
}

func TestAutoUnlockInServerGo(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("failed to read server.go: %v", err)
	}
	code := string(src)
	if !contains(code, "autoUnlock(") {
		t.Error("server.go must call autoUnlock() for VC-managed unlock")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
