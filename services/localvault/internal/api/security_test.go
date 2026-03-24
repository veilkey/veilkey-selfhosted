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

// ── Static source analysis tests ──────────────────────────────────────────────

func TestSourceSecurity_Heartbeat_HandlerExists(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "func (s *Server) StartHeartbeat(") {
		t.Error("StartHeartbeat method must be defined on Server")
	}
	if !strings.Contains(content, "func (s *Server) SendHeartbeatOnce(") {
		t.Error("SendHeartbeatOnce method must be defined on Server")
	}
}

func TestSourceSecurity_Heartbeat_SendsAuthHeader(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "agentAuthHeader()") {
		t.Error("SendHeartbeatOnce must include agent auth header for VC authentication")
	}
	if !strings.Contains(content, `"Authorization"`) {
		t.Error("SendHeartbeatOnce must set Authorization header")
	}
}

func TestSourceSecurity_Heartbeat_EncryptsAgentSecret(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "crypto.Encrypt(kek") {
		t.Error("heartbeat must encrypt agent_secret with KEK before storing")
	}
	if !strings.Contains(content, "UpdateAgentSecret") {
		t.Error("heartbeat must persist encrypted agent_secret via UpdateAgentSecret")
	}
}

func TestSourceSecurity_Heartbeat_ConsumesRegistrationToken(t *testing.T) {
	src, err := os.ReadFile("heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read heartbeat.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, `DeleteConfig("VEILKEY_REGISTRATION_TOKEN")`) {
		t.Error("heartbeat must consume (delete) registration token after successful registration")
	}
}

func TestSourceSecurity_Lifecycle_AllHandlersExist(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)
	handlers := []string{
		"func (s *Server) handleReencrypt(",
		"func (s *Server) handleActivate(",
		"func (s *Server) handleArchive(",
		"func (s *Server) handleBlock(",
		"func (s *Server) handleRevoke(",
	}
	for _, h := range handlers {
		if !strings.Contains(content, h) {
			t.Errorf("lifecycle handler missing: %s", h)
		}
	}
}

func TestSourceSecurity_Lifecycle_RoutesRegisteredWithMiddleware(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)
	routes := []struct {
		method  string
		path    string
		handler string
	}{
		{"POST", "/api/reencrypt", "handleReencrypt"},
		{"POST", "/api/activate", "handleActivate"},
		{"POST", "/api/archive", "handleArchive"},
		{"POST", "/api/block", "handleBlock"},
		{"POST", "/api/revoke", "handleRevoke"},
	}
	for _, route := range routes {
		// Each lifecycle route must have requireTrustedIP and requireUnlocked
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route.handler) && strings.Contains(line, route.path) {
				found = true
				if !strings.Contains(line, "requireTrustedIP") {
					t.Errorf("route %s %s must be wrapped with requireTrustedIP", route.method, route.path)
				}
				if !strings.Contains(line, "requireUnlocked") {
					t.Errorf("route %s %s must be wrapped with requireUnlocked", route.method, route.path)
				}
				break
			}
		}
		if !found {
			t.Errorf("route %s %s not registered with handler %s", route.method, route.path, route.handler)
		}
	}
}

func TestSourceSecurity_Lifecycle_BlockChecksStatus(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "RefStatusBlock") {
		t.Error("lifecycle handlers must check RefStatusBlock to prevent operations on blocked refs")
	}
}

func TestSourceSecurity_AdminAPI_AllRoutesTrusted(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "func (s *Server) registerAdminRoutes(mux *http.ServeMux)") {
		t.Error("registerAdminRoutes must be defined")
	}
	// Verify trusted() alias is used
	if !strings.Contains(content, "trusted := s.requireTrustedIP") {
		t.Error("registerAdminRoutes must use requireTrustedIP middleware alias")
	}
	// Each admin route line must use trusted()
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "HandleFunc") && strings.Contains(line, "/api/admin/") {
			if !strings.Contains(line, "trusted(") {
				t.Errorf("admin route must use trusted() middleware: %s", strings.TrimSpace(line))
			}
		}
		if strings.Contains(line, "HandleFunc") && strings.Contains(line, "/api/lock") {
			if !strings.Contains(line, "trusted(") {
				t.Errorf("/api/lock route must use trusted() middleware: %s", strings.TrimSpace(line))
			}
		}
	}
}

func TestSourceSecurity_AdminAPI_DiagnosticsRequiresReady(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "ready(s.handleDiagnostics)") {
		t.Error("handleDiagnostics must be wrapped with ready (requireUnlocked) middleware")
	}
}

func TestSourceSecurity_AdminAPI_LockRequiresTrustedAndReady(t *testing.T) {
	src, err := os.ReadFile("admin_api.go")
	if err != nil {
		t.Fatalf("failed to read admin_api.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "trusted(ready(s.handleLock))") {
		t.Error("handleLock must be wrapped with trusted + ready middleware")
	}
}

func TestSourceSecurity_KeycenterTarget_ResolvesFromEnvAndDB(t *testing.T) {
	src, err := os.ReadFile("keycenter_target.go")
	if err != nil {
		t.Fatalf("failed to read keycenter_target.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, `os.Getenv("VEILKEY_VAULTCENTER_URL")`) {
		t.Error("resolveVaultcenterTarget must check VEILKEY_VAULTCENTER_URL env var")
	}
	if !strings.Contains(content, "lookupConfigValue") {
		t.Error("resolveVaultcenterTarget must also check DB config")
	}
}

func TestSourceSecurity_KeycenterTarget_NormalizesURL(t *testing.T) {
	src, err := os.ReadFile("keycenter_target.go")
	if err != nil {
		t.Fatalf("failed to read keycenter_target.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "func normalizeURL(") {
		t.Error("normalizeURL helper must be defined for URL cleanup")
	}
	if !strings.Contains(content, "TrimRight") {
		t.Error("normalizeURL must trim trailing slashes")
	}
}

func TestSourceSecurity_UnlockRoute_RequiresTrustedIP(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireTrustedIP(s.unlockLimiter.Middleware(s.handleUnlock))") {
		t.Error("handleUnlock must be wrapped with requireTrustedIP + rate limiter")
	}
}

func TestSourceSecurity_AgentSecret_ConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "subtle.ConstantTimeCompare") {
		t.Error("requireAgentSecret must use constant-time comparison to prevent timing attacks")
	}
}
