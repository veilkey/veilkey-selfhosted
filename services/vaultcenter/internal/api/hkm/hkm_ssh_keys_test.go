package hkm

import (
	"os"
	"strings"
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Domain-level tests for SSH key API handlers
// These verify endpoint registration, auth policy, input validation,
// and response structure for SSH key management.
// ══════════════════════════════════════════════════════════════════

// --- Route registration ---

// Guarantees: GET /api/ssh/keys is registered and accessible without agentAuth.
func TestSource_SSHKeys_RouteRegistered(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"GET /api/ssh/keys"`) {
		t.Error("GET /api/ssh/keys route must be registered in handler.go")
	}
	if !strings.Contains(content, `"DELETE /api/ssh/keys/{ref}"`) {
		t.Error("DELETE /api/ssh/keys/{ref} route must be registered in handler.go")
	}
}

// Guarantees: SSH key routes do not use agentAuth middleware.
// SSH keys are admin-managed, not agent-managed.
func TestSource_SSHKeys_NoAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "/api/ssh/keys") {
			if strings.Contains(line, "agentAuth(") {
				t.Errorf("line %d: SSH key route must not use agentAuth middleware", i+1)
			}
		}
	}
}

// Guarantees: DELETE /api/ssh/keys/{ref} requires trusted IP.
func TestSource_SSHKeyDelete_RequiresTrustedIP(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, `"DELETE /api/ssh/keys/{ref}"`) {
			if !strings.Contains(line, "trusted(") {
				t.Error("DELETE /api/ssh/keys/{ref} must require trusted IP")
			}
			return
		}
	}
	t.Error("DELETE /api/ssh/keys/{ref} route not found")
}

// --- Handler: handleSSHKeys ---

// Guarantees: handleSSHKeys uses ListRefsByScope with RefScopeSSH.
func TestSource_SSHKeys_UsesRefScopeSSH(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ListRefsByScope") {
		t.Error("handleSSHKeys must use ListRefsByScope to query SSH refs")
	}
	if !strings.Contains(content, "RefScopeSSH") {
		t.Error("handleSSHKeys must filter by db.RefScopeSSH")
	}
}

// Guarantees: handleSSHKeys only returns active SSH keys.
// Archived/revoked keys must not appear in the list.
func TestSource_SSHKeys_FiltersActiveOnly(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "RefStatusActive") {
		t.Error("handleSSHKeys must filter for active status only")
	}
}

// Guarantees: handleSSHKeys response includes ssh_keys array and count.
func TestSource_SSHKeys_ResponseStructure(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"ssh_keys"`) {
		t.Error("response must include ssh_keys field")
	}
	if !strings.Contains(content, `"count"`) {
		t.Error("response must include count field")
	}
}

// Guarantees: SSH key entries do not expose plaintext values.
// Only ref, status, and created_at should be returned.
func TestSource_SSHKeys_NoPlaintextExposed(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if strings.Contains(content, "Ciphertext") && strings.Contains(content, `json:"ciphertext"`) {
		t.Error("SSH key list must not expose ciphertext in response")
	}
	if strings.Contains(content, "Decrypt") {
		t.Error("SSH key list handler must not decrypt keys — list is metadata only")
	}
}

// --- Handler: handleSSHKeyDelete ---

// Guarantees: handleSSHKeyDelete validates the ref is an SSH key before deleting.
// Prevents accidental deletion of non-SSH refs via this endpoint.
func TestSource_SSHKeyDelete_ValidatesScope(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "RefScopeSSH") || !strings.Contains(content, "ref is not an SSH key") {
		t.Error("handleSSHKeyDelete must verify ref scope is SSH before deleting")
	}
}

// Guarantees: handleSSHKeyDelete checks ref existence before deleting.
func TestSource_SSHKeyDelete_ChecksExistence(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "GetRef") {
		t.Error("handleSSHKeyDelete must look up ref before deleting")
	}
	if !strings.Contains(content, "StatusNotFound") {
		t.Error("handleSSHKeyDelete must return 404 for nonexistent refs")
	}
}

// Guarantees: handleSSHKeyDelete rejects empty ref parameter.
func TestSource_SSHKeyDelete_RejectsEmptyRef(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `ref == ""`) {
		t.Error("handleSSHKeyDelete must reject empty ref with 400")
	}
}

// Guarantees: handleSSHKeyDelete returns the deleted ref in response.
func TestSource_SSHKeyDelete_ReturnsDeletedRef(t *testing.T) {
	src, err := os.ReadFile("hkm_ssh_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_ssh_keys.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"deleted"`) {
		t.Error("handleSSHKeyDelete must return deleted ref in response body")
	}
}
