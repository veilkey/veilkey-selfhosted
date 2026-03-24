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
