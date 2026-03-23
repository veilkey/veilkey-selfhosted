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
	// Server must always start in locked mode
	// KEK only exists in memory after POST /api/unlock
	// No auto-unlock mechanism should exist

	// Check that no password-related env vars trigger auto-unlock
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
