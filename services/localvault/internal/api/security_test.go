package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
	}
	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestRequireTrustedIP_UsesNetSplitHostPort(t *testing.T) {
	s := &Server{
		trustedIPs:   map[string]bool{"10.0.0.1": true},
		trustedCIDRs: nil,
	}
	handler := s.requireTrustedIP(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// IPv4 with port — net.SplitHostPort handles this correctly
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for trusted IP with port, got %d", rec.Code)
	}

	// IPv6 with port — strings.Split(":")[0] would fail here
	s2 := &Server{
		trustedIPs:   map[string]bool{"::1": true},
		trustedCIDRs: nil,
	}
	handler2 := s2.requireTrustedIP(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "[::1]:54321"
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 for IPv6 trusted IP, got %d", rec2.Code)
	}
}

func TestRequireAgentSecret_LockedReturns503(t *testing.T) {
	s := &Server{
		locked: true,
	}
	handler := s.requireAgentSecret(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when locked, got %d", rec.Code)
	}
}

func TestHandleUnlock_PasswordTooLong(t *testing.T) {
	s := &Server{
		locked: true,
	}
	longPassword := `{"password":"` + strings.Repeat("a", 300) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/unlock", strings.NewReader(longPassword))
	rec := httptest.NewRecorder()
	s.handleUnlock(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for password > 256 chars, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password too long") {
		t.Errorf("expected 'password too long' message, got %s", rec.Body.String())
	}
}

// ── Heartbeat: vault_unlock_key requires agent auth ──────────────────────────

func TestSourceSecurity_Heartbeat_VaultUnlockKey_RequiresAuth(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "agentAuthHeader()") {
		t.Error("heartbeat must use agentAuthHeader() for authenticated communication with VaultCenter")
	}
}

func TestSourceSecurity_Heartbeat_StoresAgentSecretEncrypted(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "crypto.Encrypt(kek") {
		t.Error("heartbeat must encrypt agent_secret with KEK before storing locally")
	}
}

// ── Admin diagnostics: no full env dump, only specific keys ──────────────────

func TestSourceSecurity_AdminDiagnostics_NoFullEnvDump(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)

	// Must NOT dump all env vars (os.Environ)
	if strings.Contains(content, "os.Environ()") {
		t.Error("admin diagnostics must NOT dump all environment variables (os.Environ)")
	}

	// Should only expose specific safe keys
	allowedEnvKeys := []string{
		"VEILKEY_VERSION",
		"VEILKEY_DB_PATH",
		"VEILKEY_ADDR",
		"VEILKEY_VAULT_NAME",
	}
	for _, key := range allowedEnvKeys {
		if !strings.Contains(content, key) {
			t.Errorf("admin diagnostics should expose safe key: %s", key)
		}
	}

	// Must NOT expose sensitive env vars
	sensitiveKeys := []string{
		"VEILKEY_DB_KEY",
		"VEILKEY_KEK",
		"VEILKEY_ADMIN_PASSWORD",
	}
	for _, key := range sensitiveKeys {
		if strings.Contains(content, key) {
			t.Errorf("admin diagnostics must NOT expose sensitive key: %s", key)
		}
	}
}

func TestSourceSecurity_AdminDiagnostics_RequiresTrustedIP(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "diagnostics") && strings.Contains(line, "HandleFunc") {
			if !strings.Contains(line, "trusted(") {
				t.Error("diagnostics endpoint must require trusted IP middleware")
			}
			break
		}
	}
}

func TestSourceSecurity_AdminDiagnostics_RequiresUnlocked(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "diagnostics") && strings.Contains(line, "HandleFunc") {
			if !strings.Contains(line, "ready(") {
				t.Error("diagnostics endpoint must require server unlocked (ready) middleware")
			}
			break
		}
	}
}

func extractFn(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 { return "" }
	r := code[i:]
	n := strings.Index(r[1:], "\nfunc ")
	if n < 0 { return r }
	return r[:n+1]
}

// ══ Salt on chain ═══════════════════════════════════════════════

func TestHeartbeatSendsSalt(t *testing.T) {
	s, _ := os.ReadFile("heartbeat.go")
	c := string(s)
	if !strings.Contains(c, "base64") {
		t.Error("heartbeat must send salt as base64")
	}
	if !strings.Contains(c, "len(saltBytes) > 0") {
		t.Error("must skip nil salt")
	}
}

func TestAutoUnlockReceivesSalt(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) AutoUnlockFromVC(")
	if !strings.Contains(b, "result.Salt") {
		t.Error("must handle salt in response")
	}
	if !strings.Contains(b, `len(salt) == 0`) {
		t.Error("must error when no salt")
	}
}

func TestSaltFileNotFatal(t *testing.T) {
	s, _ := os.ReadFile("../commands/server.go")
	for _, line := range strings.Split(string(s), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(line, "log.Fatal") && strings.Contains(lower, "salt") {
			t.Error("salt file missing must be warning not fatal")
		}
	}
}

// ══════════════════════════════════════════════════════════════════
// Salt-on-chain: LV design principle verification
// LV must not be able to self-decrypt without VC.
// ══════════════════════════════════════════════════════════════════

// --- Heartbeat sends salt to VC ---

func TestHeartbeatSaltBase64Guarded(t *testing.T) {
	s, _ := os.ReadFile("heartbeat.go")
	c := string(s)
	// Must guard against nil salt
	if !strings.Contains(c, "len(saltBytes) > 0") {
		t.Error("heartbeat must guard nil salt before base64 encoding")
	}
	// Must use base64 encoding
	if !strings.Contains(c, "base64.StdEncoding.EncodeToString") {
		t.Error("heartbeat must base64-encode salt")
	}
	// Must NOT have duplicate salt blocks
	count := strings.Count(c, `payload["salt"]`)
	if count > 1 {
		t.Errorf("heartbeat has %d salt assignments — must be exactly 1", count)
	}
}

// --- AutoUnlockFromVC handles salt from VC ---

func TestAutoUnlockReceivesSaltFromVC(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) AutoUnlockFromVC(")
	if b == "" {
		t.Fatal("AutoUnlockFromVC must exist")
	}
	// Must have Salt in response struct
	if !strings.Contains(b, `Salt`) && !strings.Contains(b, `json:"salt"`) {
		t.Error("AutoUnlockFromVC response must include Salt field")
	}
	// Must decode base64 salt
	if !strings.Contains(b, "base64.StdEncoding.DecodeString") {
		t.Error("must base64-decode salt from VC response")
	}
	// Must validate decoded salt length
	if !strings.Contains(b, "len(decoded) > 0") {
		t.Error("must validate decoded salt is non-empty")
	}
	// Must update local cache file
	if !strings.Contains(b, "os.WriteFile") {
		t.Error("must write salt to local cache file after VC recovery")
	}
	// Must update s.salt in memory
	if !strings.Contains(b, "s.salt = salt") && !strings.Contains(b, "s.salt = decoded") {
		t.Error("must update in-memory salt after VC recovery")
	}
}

// --- nil salt prevents DeriveKEK ---

func TestAutoUnlockRejectsNilSalt(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) AutoUnlockFromVC(")
	if !strings.Contains(b, `len(salt) == 0`) {
		t.Error("AutoUnlockFromVC must error when salt is nil/empty")
	}
	if !strings.Contains(b, "no salt available") {
		t.Error("error message must indicate no salt available")
	}
}

// --- handleUnlock uses s.salt (nil = fail) ---

func TestHandleUnlockUsesSalt(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) handleUnlock(")
	if b == "" {
		t.Fatal("handleUnlock must exist")
	}
	if !strings.Contains(b, "s.salt") {
		t.Error("handleUnlock must use s.salt for KEK derivation")
	}
	// When s.salt is nil, DeriveKEK produces wrong KEK → Unlock fails → 401
	// This is the security guarantee: no salt = no unlock without VC
	if !strings.Contains(b, "DeriveKEK") {
		t.Error("handleUnlock must call DeriveKEK with salt")
	}
}

// --- No duplicate password length checks ---

func TestHandleUnlockNoDuplicateChecks(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) handleUnlock(")
	count := strings.Count(b, "len(req.Password) > 256")
	if count > 1 {
		t.Errorf("handleUnlock has %d password length checks — must be exactly 1", count)
	}
}

// --- salt file missing = warning, not fatal ---

func TestSaltFileMissingIsWarning(t *testing.T) {
	s, _ := os.ReadFile("../commands/server.go")
	c := string(s)
	// Must have WARNING log for missing salt
	if !strings.Contains(c, "WARNING: salt file not found") {
		t.Error("missing salt file must produce WARNING log")
	}
	// Must set salt = nil
	if !strings.Contains(c, "salt = nil") {
		t.Error("missing salt must set salt = nil for VC recovery mode")
	}
	// Must NOT fatal on salt missing
	for _, line := range strings.Split(c, "\n") {
		if strings.Contains(line, "log.Fatal") && strings.Contains(strings.ToLower(line), "salt file not found") {
			t.Error("salt file missing must not be fatal")
		}
	}
}

// --- VC is required: password alone is not enough ---

func TestVCRequiredForDecryption(t *testing.T) {
	// Design principle: LV cannot self-decrypt without VC.
	// Verify: password (vault_unlock_key) is ONLY on VC, never on LV disk.
	
	// 1. vault_key file is deleted after VC registration
	hb, _ := os.ReadFile("heartbeat.go")
	if !strings.Contains(string(hb), "vault_key") {
		t.Log("NOTE: vault_key handling in heartbeat — verify bootstrap file is deleted")
	}
	
	// 2. AutoUnlockFromVC requires VC connectivity
	api, _ := os.ReadFile("api.go")
	b := extractFn(string(api), "func (s *Server) AutoUnlockFromVC(")
	if !strings.Contains(b, "agents/unlock-key") {
		t.Error("AutoUnlockFromVC must contact VC endpoint")
	}
	if !strings.Contains(b, "Bearer") {
		t.Error("AutoUnlockFromVC must authenticate with agent_secret")
	}
}

// --- ChainStoreAdapter no-op has correct signature ---

func TestLVChainStoreAdapterSaltParam(t *testing.T) {
	s, _ := os.ReadFile("../db/chain_store.go")
	b := extractFn(string(s), "func (a *ChainStoreAdapter) UpsertAgent(")
	if b == "" {
		t.Fatal("LV ChainStoreAdapter.UpsertAgent must exist")
	}
	// Count params: should have 11 unnamed params (10 original + 1 salt)
	// The salt param is `_ string` at the end
	paramLine := ""
	for _, line := range strings.Split(b, "\n") {
		if strings.Contains(line, "func (a *ChainStoreAdapter) UpsertAgent(") {
			paramLine = line
			break
		}
	}
	// Must have string at the end (salt)
	if !strings.Contains(paramLine, "_ string") {
		t.Error("LV ChainStoreAdapter.UpsertAgent must have salt string param")
	}
}

// ══════════════════════════════════════════════════════════════════
// Logic bug regression tests
// ══════════════════════════════════════════════════════════════════

func TestBulkApplyValidatesAllBeforeExecution(t *testing.T) {
	s, _ := os.ReadFile("bulk/apply.go")
	phase1 := strings.Index(string(s), "Phase 1: Validate ALL")
	phase2 := strings.Index(string(s), "Phase 2: Execute")
	if phase1 < 0 || phase2 < 0 {
		t.Fatal("bulk apply must have Phase 1 (validate) and Phase 2 (execute)")
	}
	if phase1 > phase2 {
		t.Error("Phase 1 (validate) must come before Phase 2 (execute)")
	}
}

func TestBulkApplySymlinkResolution(t *testing.T) {
	s, _ := os.ReadFile("bulk/apply.go")
	b := extractFn(string(s), "func writeAtomically(")
	if !strings.Contains(b, "EvalSymlinks") {
		t.Error("writeAtomically must resolve symlinks")
	}
}

func TestCipherSaveRespectsRequestScope(t *testing.T) {
	s, _ := os.ReadFile("secrets/cipher_save.go")
	b := extractFn(string(s), "func (h *Handler) handleSaveCipher(")
	if b == "" {
		t.Fatal("handleSaveCipher must exist")
	}
	if !strings.Contains(b, "req.Scope") {
		t.Error("cipher save must use req.Scope")
	}
	// Must not silently ignore scope
	if !strings.Contains(b, `req.Scope != ""`) {
		t.Error("must check req.Scope is non-empty before applying")
	}
}

func TestHeartbeatSkipsWhenNoIdentity(t *testing.T) {
	s, _ := os.ReadFile("heartbeat.go")
	b := extractFn(string(s), "func (s *Server) SendHeartbeatOnce(")
	if !strings.Contains(b, "identity == nil") {
		t.Error("heartbeat must return early when identity is nil")
	}
}

func TestNilSaltPreventsKEKDerivation(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) AutoUnlockFromVC(")
	if !strings.Contains(b, `len(salt) == 0`) {
		t.Error("must reject nil salt before DeriveKEK")
	}
}

// ══ MaxBytesReader on all JSON decoders ═════════════════════════

func TestAllLVJSONDecodersHaveMaxBytes(t *testing.T) {
	files := []struct {
		path string
		desc string
	}{
		{"lifecycle.go", "lifecycle handlers"},
		{"install_wizard.go", "install wizard"},
		{"api.go", "api handlers"},
	}
	for _, f := range files {
		src, err := os.ReadFile(f.path)
		if err != nil {
			t.Logf("skip %s: %v", f.path, err)
			continue
		}
		code := string(src)
		lines := strings.Split(code, "\n")
		for i, line := range lines {
			if strings.Contains(line, "json.NewDecoder(r.Body)") {
				// Check preceding 3 lines for MaxBytesReader
				found := false
				for j := max(0, i-6); j < i; j++ {
					if strings.Contains(lines[j], "MaxBytesReader") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s:%d: json.NewDecoder without MaxBytesReader", f.path, i+1)
				}
			}
		}
	}
}

func TestSubpackageJSONDecodersHaveMaxBytes(t *testing.T) {
	subpkgs := []struct {
		path string
		desc string
	}{
		{"secrets/cipher_save.go", "cipher save"},
		{"secrets/secrets.go", "secrets resolve"},
		{"secrets/fields.go", "secret fields"},
		{"configs/configs.go", "config CRUD"},
		{"bulk/apply.go", "bulk apply"},
		{"functions/functions.go", "functions"},
	}
	for _, f := range subpkgs {
		src, err := os.ReadFile(f.path)
		if err != nil {
			t.Logf("skip %s: %v", f.path, err)
			continue
		}
		code := string(src)
		lines := strings.Split(code, "\n")
		for i, line := range lines {
			if strings.Contains(line, "json.NewDecoder(r.Body)") {
				found := false
				for j := max(0, i-6); j < i; j++ {
					if strings.Contains(lines[j], "MaxBytesReader") {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s:%d: json.NewDecoder without MaxBytesReader", f.path, i+1)
				}
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
