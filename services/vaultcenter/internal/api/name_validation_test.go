package api

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis tests for name_validation.go ──────────────────────────────
// The isValidResourceName function is unexported and delegates to
// httputil.IsValidResourceName from the shared package. These tests verify
// the delegation is correct and the function is used where expected.

func TestSourceNameValidation_DelegatesToSharedPackage(t *testing.T) {
	src, err := os.ReadFile("name_validation.go")
	if err != nil {
		t.Fatalf("failed to read name_validation.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func isValidResourceName(name string) bool") {
		t.Error("isValidResourceName must be defined as a package-level function")
	}
	if !strings.Contains(content, "httputil.IsValidResourceName(name)") {
		t.Error("isValidResourceName must delegate to httputil.IsValidResourceName")
	}
}

func TestSourceNameValidation_ImportsHttputil(t *testing.T) {
	src, err := os.ReadFile("name_validation.go")
	if err != nil {
		t.Fatalf("failed to read name_validation.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"veilkey-vaultcenter/internal/httputil"`) {
		t.Error("name_validation.go must import internal httputil package")
	}
}

func TestSourceNameValidation_UsedInHandlers(t *testing.T) {
	// Verify that isValidResourceName is actually called somewhere in the api package.
	// Check the HKM helpers where it's typically used for secret/config name validation.
	src, err := os.ReadFile("hkm/helpers.go")
	if err != nil {
		t.Fatalf("failed to read hkm/helpers.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "IsValidResourceName") {
		t.Error("IsValidResourceName must be used in hkm helpers for input validation")
	}
}

func TestSourceNameValidation_SharedPackageReExported(t *testing.T) {
	// Verify that the internal httputil re-exports the shared package's function
	src, err := os.ReadFile("../httputil/httputil.go")
	if err != nil {
		t.Fatalf("failed to read httputil/httputil.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func IsValidResourceName(name string) bool") {
		t.Error("httputil must export IsValidResourceName")
	}
	if !strings.Contains(content, "sharedhttp.IsValidResourceName(name)") {
		t.Error("httputil.IsValidResourceName must delegate to sharedhttp package")
	}
}
