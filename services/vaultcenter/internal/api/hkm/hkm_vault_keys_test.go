package hkm

import (
	"os"
	"strings"
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Domain-level tests for vault key handlers
// These verify that /api/vaults/ endpoints never require agent auth
// and that the vault-proxy bypass is correctly implemented.
// ══════════════════════════════════════════════════════════════════

// --- Bug regression: vault handlers must NOT call verifyAgentAccess directly ---

// Guarantees: handleVaultKeys does not gate on verifyAgentAccess.
// Without this fix, TUI/admin requests to /api/vaults/{vault}/keys
// always returned 403 because no agent Bearer token was present.
func TestSource_VaultKeys_NoAgentAccessCheck(t *testing.T) {
	src, err := os.ReadFile("hkm_vault_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_vault_keys.go: %v", err)
	}
	content := string(src)

	// No vault handler should call verifyAgentAccess directly
	if strings.Contains(content, "verifyAgentAccess") {
		t.Error("hkm_vault_keys.go must not call verifyAgentAccess — vault endpoints are admin-initiated, not agent-initiated")
	}
}

// --- Vault-proxy bypass: asVaultProxy must exist and inject context ---

// Guarantees: asVaultProxy function exists and injects the vault-proxy context key.
// This allows agent handlers to accept requests forwarded from vault handlers.
func TestSource_AsVaultProxy_Exists(t *testing.T) {
	src, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("failed to read helpers.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func asVaultProxy(") {
		t.Error("helpers.go must define asVaultProxy() function")
	}
	if !strings.Contains(content, "vaultProxyKey") {
		t.Error("asVaultProxy must use vaultProxyKey context key")
	}
	if !strings.Contains(content, `SetPathValue("agent"`) {
		t.Error("asVaultProxy must set 'agent' path value from 'vault'")
	}
}

// Guarantees: verifyAgentAccess respects the vault-proxy bypass flag.
func TestSource_VerifyAgentAccess_RespectsVaultProxy(t *testing.T) {
	src, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("failed to read helpers.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "vaultProxyKey") {
		t.Error("verifyAgentAccess must check vaultProxyKey context")
	}
	// Must return true for proxy requests before checking agent auth
	lines := strings.Split(content, "\n")
	proxyCheckFound := false
	agentCheckFound := false
	for _, line := range lines {
		if strings.Contains(line, "vaultProxyKey") && strings.Contains(line, "bypass") {
			proxyCheckFound = true
		}
		if strings.Contains(line, "agentAuthKey") && proxyCheckFound {
			agentCheckFound = true
		}
	}
	if !proxyCheckFound {
		t.Error("verifyAgentAccess must check vault-proxy bypass flag")
	}
	if !agentCheckFound {
		t.Error("vault-proxy bypass must be checked BEFORE agent auth check")
	}
}

// --- All vault→agent delegations must use asVaultProxy ---

// Guarantees: Every vault handler that delegates to an agent handler
// uses asVaultProxy() instead of raw SetPathValue("agent", ...).
func TestSource_VaultHandlers_UseAsVaultProxy(t *testing.T) {
	src, err := os.ReadFile("hkm_vault_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_vault_keys.go: %v", err)
	}
	content := string(src)

	// All agent handler delegations must go through asVaultProxy
	delegatedHandlers := []string{
		"handleAgentSecrets",
		"handleAgentGetSecret",
		"handleAgentSaveSecret",
		"handleAgentDeleteSecret",
		"handleAgentSaveSecretFields",
		"handleAgentGetSecretField",
		"handleAgentDeleteSecretField",
	}

	for _, handler := range delegatedHandlers {
		if !strings.Contains(content, handler) {
			continue // handler not delegated, skip
		}
		// Find all lines that call this handler
		for i, line := range strings.Split(content, "\n") {
			if strings.Contains(line, "h."+handler+"(w,") {
				// Must use asVaultProxy(r) directly or a variable derived from it (pr)
				if !strings.Contains(line, "asVaultProxy") && !strings.Contains(line, ", pr)") {
					t.Errorf("line %d: h.%s must be called with asVaultProxy(r) or proxy var, not raw r", i+1, handler)
				}
			}
		}
	}
}

// --- Route registration: vault endpoints must NOT use agentAuth middleware ---

// Guarantees: /api/vaults/ routes are registered with ready() middleware only,
// not wrapped with agentAuth(). agentAuth would require Bearer token from agents.
func TestSource_VaultRoutes_NoAgentAuthMiddleware(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "/api/vaults/") {
			if strings.Contains(line, "agentAuth(") {
				t.Errorf("line %d: vault route must not use agentAuth middleware: %s", i+1, strings.TrimSpace(line))
			}
		}
	}
}

// --- Agent endpoints must still require agentAuth ---

// Guarantees: /api/agents/{agent}/secrets and similar agent-scoped endpoints
// are still protected by agentAuth middleware. The vault-proxy bypass only
// works when called from vault handlers, not from direct agent routes.
func TestSource_AgentRoutes_RequireAgentAuth(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	agentRoutes := []string{
		"/api/agents/{agent}/secrets\"",
		"/api/agents/{agent}/secrets/{name}\"",
		"/api/agents/{agent}/configs\"",
	}

	for _, route := range agentRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "agentAuth(") {
					t.Errorf("agent route %s must use agentAuth middleware", route)
				}
			}
		}
		if !found {
			t.Errorf("agent route %s not found in handler.go", route)
		}
	}
}

// --- Security: vault-proxy must not be injectable from external requests ---

// Guarantees: The vault-proxy context key is a private unexported type,
// preventing external callers from injecting it into request context.
func TestSource_VaultProxyKey_IsUnexported(t *testing.T) {
	src, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatalf("failed to read helpers.go: %v", err)
	}
	content := string(src)

	// Context key type must be unexported (lowercase)
	if !strings.Contains(content, "type vaultProxyContextKey struct{}") {
		t.Error("vaultProxyContextKey must be a private unexported struct type")
	}
	// Key variable must be unexported
	if !strings.Contains(content, "var vaultProxyKey = vaultProxyContextKey{}") {
		t.Error("vaultProxyKey must be an unexported package-level variable")
	}
}

// --- Proxy variable must originate from asVaultProxy ---

// Guarantees: When a vault handler uses a "pr" variable to call agent handlers,
// that variable must be assigned from asVaultProxy(r).
func TestSource_ProxyVar_FromAsVaultProxy(t *testing.T) {
	src, err := os.ReadFile("hkm_vault_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_vault_keys.go: %v", err)
	}
	content := string(src)

	// Every "pr :=" or "pr =" must come from asVaultProxy
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pr :=") || strings.HasPrefix(trimmed, "pr =") {
			if !strings.Contains(line, "asVaultProxy") {
				t.Errorf("line %d: proxy variable 'pr' must be assigned from asVaultProxy(r)", i+1)
			}
		}
	}
}

// --- Completeness: all SetPathValue("agent") in vault handlers must be removed ---

// Guarantees: No vault handler uses the old raw SetPathValue("agent", ...) pattern
// which bypasses asVaultProxy and thus misses the vault-proxy context flag.
func TestSource_NoRawSetPathValueAgent(t *testing.T) {
	src, err := os.ReadFile("hkm_vault_keys.go")
	if err != nil {
		t.Fatalf("failed to read hkm_vault_keys.go: %v", err)
	}
	content := string(src)

	// The old pattern: r.SetPathValue("agent", r.PathValue("vault"))
	if strings.Contains(content, `r.SetPathValue("agent"`) {
		t.Error("hkm_vault_keys.go must not use raw r.SetPathValue(\"agent\", ...) — use asVaultProxy(r) instead")
	}
}
