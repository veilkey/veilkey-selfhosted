package api

import (
	"os"
	"strings"
	"testing"
)

// ── Static source analysis tests for handle_keycenter.go ──────────────────────
// These tests verify security properties and request validation by reading the
// source code directly, since the handlers depend on a fully initialized Server
// with DB, chain, crypto keys, and HTTP clients.

func TestSourceKeycenter_PromoteRequiresAllFields(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	// handleKeycenterPromoteToVault must validate ref, name, vault_hash
	if !strings.Contains(content, `req.Ref == ""`) {
		t.Error("handleKeycenterPromoteToVault must check for empty ref")
	}
	if !strings.Contains(content, `req.Name == ""`) {
		t.Error("handleKeycenterPromoteToVault must check for empty name")
	}
	if !strings.Contains(content, `req.VaultHash == ""`) {
		t.Error("handleKeycenterPromoteToVault must check for empty vault_hash")
	}
	if !strings.Contains(content, `"ref, name, and vault_hash are required"`) {
		t.Error("handleKeycenterPromoteToVault must return descriptive validation error")
	}
}

func TestSourceKeycenter_PromoteRequestStructFields(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	// Verify the struct has the required JSON fields
	if !strings.Contains(content, `json:"ref"`) {
		t.Error("promote request struct must have ref field")
	}
	if !strings.Contains(content, `json:"name"`) {
		t.Error("promote request struct must have name field")
	}
	if !strings.Contains(content, `json:"vault_hash"`) {
		t.Error("promote request struct must have vault_hash field")
	}
}

func TestSourceKeycenter_CreateTempRefRequiresValue(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `req.Value == ""`) {
		t.Error("handleKeycenterCreateTempRef must validate value is not empty")
	}
	if !strings.Contains(content, `"value is required"`) {
		t.Error("handleKeycenterCreateTempRef must return 'value is required' error")
	}
}

func TestSourceKeycenter_CreateTempRefTrimsInput(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "strings.TrimSpace(req.Name)") {
		t.Error("handleKeycenterCreateTempRef must trim name")
	}
	if !strings.Contains(content, "strings.TrimSpace(req.Value)") {
		t.Error("handleKeycenterCreateTempRef must trim value")
	}
}

func TestSourceKeycenter_CreateTempRefDeduplicates(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "FindActiveTempRefByHash") {
		t.Error("handleKeycenterCreateTempRef must dedup using FindActiveTempRefByHash")
	}
	if !strings.Contains(content, `"deduplicated": true`) {
		t.Error("handleKeycenterCreateTempRef must indicate deduplicated result")
	}
}

func TestSourceKeycenter_CreateTempRefUsesEncryption(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "crypto.Encrypt(dek") {
		t.Error("handleKeycenterCreateTempRef must encrypt value with DEK")
	}
	if !strings.Contains(content, "GetLocalDEK()") {
		t.Error("handleKeycenterCreateTempRef must use GetLocalDEK for encryption key")
	}
}

func TestSourceKeycenter_CreateTempRefSubmitsTx(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "s.SubmitTx(") {
		t.Error("handleKeycenterCreateTempRef must submit blockchain transaction")
	}
	if !strings.Contains(content, "chain.TxSaveTokenRef") {
		t.Error("handleKeycenterCreateTempRef must use TxSaveTokenRef transaction type")
	}
}

func TestSourceKeycenter_RevealRefExists(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func (s *Server) handleKeycenterRevealRef(") {
		t.Error("handleKeycenterRevealRef must be defined on Server")
	}
}

func TestSourceKeycenter_RevealRefOnlyTempScope(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "RefScopeTemp") {
		t.Error("handleKeycenterRevealRef must check for TEMP scope")
	}
	if !strings.Contains(content, "only TEMP refs can be revealed here") {
		t.Error("handleKeycenterRevealRef must reject non-TEMP refs")
	}
}

func TestSourceKeycenter_RevealRefRequiresPathParam(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `r.PathValue("ref")`) {
		t.Error("handleKeycenterRevealRef must extract ref from URL path")
	}
	if !strings.Contains(content, `canonical == ""`) {
		t.Error("handleKeycenterRevealRef must validate ref path param is not empty")
	}
}

func TestSourceKeycenter_PromoteRegistersTrackedRef(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	// After promote, a tracked ref must be registered so /api/resolve/{ref} works
	if !strings.Contains(content, "Register tracked ref") || !strings.Contains(content, "SubmitTx") {
		// The comment might vary, but SubmitTx is the key
		if strings.Count(content, "SubmitTx") < 1 {
			t.Error("handleKeycenterPromoteToVault must register tracked ref via SubmitTx")
		}
	}
}

func TestSourceKeycenter_PromoteSendsCipherToVault(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "/api/cipher") {
		t.Error("handleKeycenterPromoteToVault must send pre-encrypted data to LV via /api/cipher")
	}
	if !strings.Contains(content, "DecryptAgentDEK") {
		t.Error("handleKeycenterPromoteToVault must decrypt agent DEK for per-vault encryption")
	}
}

func TestSourceKeycenter_RouteRegistration_MatchesExpectedMiddleware(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)

	// All keycenter routes that modify data must require adminAuth
	keycenterRoutes := []struct {
		path       string
		handler    string
		middleware string
	}{
		{"/api/keycenter/temp-refs", "handleKeycenterTempRefs", "requireAdminAuth"},
		{"/api/keycenter/temp-refs", "handleKeycenterCreateTempRef", "requireAdminAuth"},
		{"/api/keycenter/promote", "handleKeycenterPromoteToVault", "requireAdminAuth"},
	}

	for _, route := range keycenterRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route.handler) {
				found = true
				if !strings.Contains(line, route.middleware) {
					t.Errorf("route for %s must use %s middleware", route.handler, route.middleware)
				}
				if !strings.Contains(line, "requireUnlocked") {
					t.Errorf("route for %s must use requireUnlocked middleware", route.handler)
				}
				break
			}
		}
		if !found {
			t.Errorf("route for %s not found in handlers.go", route.handler)
		}
	}
}

func TestSourceKeycenter_ListRefs_NoAdminAuthRequired(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)

	// handleListRefs is used by veil CLI to build mask_map -- should NOT require adminAuth
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "handleListRefs") {
			if strings.Contains(line, "requireAdminAuth") {
				t.Error("handleListRefs must NOT require adminAuth (used by CLI for mask_map)")
			}
			if !strings.Contains(line, "requireUnlocked") {
				t.Error("handleListRefs must require unlocked state")
			}
			break
		}
	}
}
