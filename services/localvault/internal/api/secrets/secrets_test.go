package secrets

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: secrets.go — /api/resolve/{ref} returns 403 ──────────────

func TestSource_ResolveSecretReturns403(t *testing.T) {
	src, err := os.ReadFile("secrets.go")
	if err != nil {
		t.Fatalf("failed to read secrets.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleResolveSecret(")
	if !strings.Contains(fnBody, "StatusForbidden") {
		t.Error("handleResolveSecret must return StatusForbidden (403)")
	}
	if !strings.Contains(fnBody, "plaintext resolution is disabled") || !strings.Contains(fnBody, "localvault direct") {
		t.Error("handleResolveSecret must state that localvault direct plaintext resolution is disabled")
	}
}

func TestSource_SaveSecretReturns403(t *testing.T) {
	src, err := os.ReadFile("secrets.go")
	if err != nil {
		t.Fatalf("failed to read secrets.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveSecret(")
	if !strings.Contains(fnBody, "StatusForbidden") {
		t.Error("handleSaveSecret must return StatusForbidden (403)")
	}
}

func TestSource_GetSecretReturns403(t *testing.T) {
	src, err := os.ReadFile("secrets.go")
	if err != nil {
		t.Fatalf("failed to read secrets.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleGetSecret(")
	if !strings.Contains(fnBody, "StatusForbidden") {
		t.Error("handleGetSecret must return StatusForbidden (403)")
	}
}

// ── Source analysis: handler.go — cipher routes require agentAuth ─────────────

func TestSource_CipherGetRequiresAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "GET /api/cipher/{ref}") && !strings.Contains(line, "fields") {
			if !strings.Contains(line, "agentAuth(") {
				t.Error("GET /api/cipher/{ref} must be wrapped with agentAuth")
			}
			return
		}
	}
	t.Error("GET /api/cipher/{ref} route not found")
}

func TestSource_CipherPostRequiresAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "POST /api/cipher") && !strings.Contains(line, "{ref}") {
			if !strings.Contains(line, "agentAuth(") {
				t.Error("POST /api/cipher must be wrapped with agentAuth")
			}
			return
		}
	}
	t.Error("POST /api/cipher route not found")
}

func TestSource_CipherFieldRequiresAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "GET /api/cipher/{ref}/fields/{field}") {
			if !strings.Contains(line, "agentAuth(") {
				t.Error("GET /api/cipher/{ref}/fields/{field} must be wrapped with agentAuth")
			}
			return
		}
	}
	t.Error("GET /api/cipher/{ref}/fields/{field} route not found")
}

// ── Source analysis: cipher_save.go — secret ref generation ──────────────────

func TestSource_SecretRefUsesGenerateHexRef(t *testing.T) {
	src, err := os.ReadFile("cipher_save.go")
	if err != nil {
		t.Fatalf("failed to read cipher_save.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `crypto.GenerateHexRef(8)`) {
		t.Error("secret ref generation must use crypto.GenerateHexRef(8)")
	}
}

func TestSource_CipherSaveValidatesName(t *testing.T) {
	src, err := os.ReadFile("cipher_save.go")
	if err != nil {
		t.Fatalf("failed to read cipher_save.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveCipher(")
	if !strings.Contains(fnBody, "isValidResourceName(req.Name)") {
		t.Error("handleSaveCipher must validate name with isValidResourceName")
	}
}

func TestSource_CipherSaveRequiresCiphertextAndNonce(t *testing.T) {
	src, err := os.ReadFile("cipher_save.go")
	if err != nil {
		t.Fatalf("failed to read cipher_save.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveCipher(")
	if !strings.Contains(fnBody, "len(req.Ciphertext) == 0") {
		t.Error("handleSaveCipher must require non-empty ciphertext")
	}
	if !strings.Contains(fnBody, "len(req.Nonce) == 0") {
		t.Error("handleSaveCipher must require non-empty nonce")
	}
}

// ── Source analysis: fields.go — field management ─────────────────────────────

func TestSource_SecretFieldsRequireActiveLifecycle(t *testing.T) {
	src, err := os.ReadFile("fields.go")
	if err != nil {
		t.Fatalf("failed to read fields.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveSecretFields(")
	if !strings.Contains(fnBody, "refStatusActive") {
		t.Error("handleSaveSecretFields must check for active status")
	}
	if !strings.Contains(fnBody, "VK:LOCAL or VK:EXTERNAL active lifecycle") {
		t.Error("handleSaveSecretFields must enforce VK:LOCAL or VK:EXTERNAL scope")
	}
}

func TestSource_DeleteFieldRequiresActiveLifecycle(t *testing.T) {
	src, err := os.ReadFile("fields.go")
	if err != nil {
		t.Fatalf("failed to read fields.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleDeleteSecretField(")
	if !strings.Contains(fnBody, "refStatusActive") {
		t.Error("handleDeleteSecretField must check for active status")
	}
}

func TestSource_FieldTypeNormalization(t *testing.T) {
	src, err := os.ReadFile("fields.go")
	if err != nil {
		t.Fatalf("failed to read fields.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func normalizeSecretFieldType(") {
		t.Error("normalizeSecretFieldType function must exist")
	}
}

func TestUnit_NormalizeSecretFieldType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"login", "login"},
		{"otp", "otp"},
		{"password", "password"},
		{"key", "key"},
		{"url", "url"},
		{"unknown", "text"},
		{"", "text"},
	}
	for _, tt := range tests {
		got := normalizeSecretFieldType(tt.input)
		if got != tt.want {
			t.Errorf("normalizeSecretFieldType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── Source analysis: handler.go — catalog/meta endpoint exists ────────────────

func TestSource_SecretMetaEndpointExists(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "GET /api/secrets/meta/{name}") {
		t.Error("secret meta endpoint GET /api/secrets/meta/{name} must be registered")
	}
}

func TestSource_RekeyEndpointRequiresAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "POST /api/rekey") {
			if !strings.Contains(line, "agentAuth(") {
				t.Error("POST /api/rekey must be wrapped with agentAuth")
			}
			return
		}
	}
	t.Error("POST /api/rekey route not found")
}

// ── Source analysis: secrets.go — resolve route is registered ─────────────────

func TestSource_ResolveRouteRegistered(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "GET /api/resolve/{ref}") {
		t.Error("GET /api/resolve/{ref} route must be registered")
	}
}

// ── Source analysis: helpers.go — vaultcenter message constant ────────────────

func TestSource_VaultcenterOnlyDecryptMessage(t *testing.T) {
	src, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("failed to read helpers.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "vaultcenterOnlyDecryptMessage") {
		t.Error("vaultcenterOnlyDecryptMessage constant must be defined")
	}
	if !strings.Contains(content, "localvault direct plaintext handling is disabled") {
		t.Error("vaultcenterOnlyDecryptMessage must state plaintext handling is disabled")
	}
}

// ── Source analysis: handler.go — fields routes require agentAuth ─────────────

func TestSource_FieldsRoutesRequireAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	fieldRoutes := []string{
		"POST /api/secrets/fields",
		"DELETE /api/secrets/{name}/fields/{field}",
	}
	for _, route := range fieldRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "agentAuth(") {
					t.Errorf("field route %s must be wrapped with agentAuth", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("field route not registered: %s", route)
		}
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

func extractFn(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 {
		return ""
	}
	rest := code[i:]
	next := strings.Index(rest[1:], "\nfunc ")
	if next < 0 {
		return rest
	}
	return rest[:next+1]
}
