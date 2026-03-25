package hkm

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: hkm_ssh.go ──────────────────────────────────

func TestSource_SSH_CreateRequiresPublicKey(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "public_key is required") {
		t.Error("SSH create must validate public_key is not empty")
	}
}

func TestSource_SSH_CreateRequiresKeyType(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "key_type is required") {
		t.Error("SSH create must validate key_type is not empty")
	}
}

func TestSource_SSH_CreateEncryptsPrivateKeyWithKEK(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "crypto.Encrypt") {
		t.Error("SSH create must encrypt private key with KEK using crypto.Encrypt")
	}
	if !strings.Contains(content, "GetKEK") {
		t.Error("SSH create must use GetKEK to retrieve encryption key")
	}
}

func TestSource_SSH_CreateGeneratesRefFromFingerprint(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "VK:SSH:") {
		t.Error("SSH create must generate ref with VK:SSH: prefix")
	}
	if !strings.Contains(content, "sha256.Sum256") {
		t.Error("SSH create must use SHA-256 for fingerprint/ref generation")
	}
}

func TestSource_SSH_DecryptUsesKEK(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "crypto.Decrypt") {
		t.Error("SSH decrypt must use crypto.Decrypt to decrypt private key")
	}
}

func TestSource_SSH_DecryptRejectsExternalKeys(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "no private key stored") {
		t.Error("SSH decrypt must reject external keys that have no private key")
	}
}

func TestSource_SSH_DecryptRejectsWhenLocked(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "server is locked") {
		t.Error("SSH operations must reject requests when server is locked (no KEK)")
	}
}

func TestSource_SSH_DuplicateFingerprintHandled(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "UNIQUE") {
		t.Error("SSH create must handle duplicate fingerprint (unique index violation)")
	}
	if !strings.Contains(content, "already exists") {
		t.Error("SSH create must return friendly error for duplicate fingerprint")
	}
}

// ── Route registration: handler.go ──────────────────────────────

func TestSource_SSH_RoutesRegistered(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	routes := []string{
		"POST /api/ssh/keys",
		"GET /api/ssh/keys",
		"GET /api/ssh/keys/{ref}",
		"DELETE /api/ssh/keys/{ref}",
		"POST /api/ssh/keys/{ref}/decrypt",
		"PUT /api/ssh/keys/{ref}/hosts",
	}
	for _, route := range routes {
		if !strings.Contains(content, route) {
			t.Errorf("handler.go must register route: %s", route)
		}
	}
}

func TestSource_SSH_DecryptRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	// The decrypt route must use the admin middleware
	if !strings.Contains(content, "admin(trusted(ready(h.handleSSHKeyDecrypt)))") {
		t.Error("SSH decrypt route must require admin auth (admin middleware)")
	}
}
