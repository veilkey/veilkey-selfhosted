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

func TestDecodeJSON_MaxBodySize(t *testing.T) {
	// Build a body larger than 1 MiB
	bigBody := strings.Repeat("x", 1<<20+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))
	var dst map[string]any
	err := decodeJSON(req, &dst)
	if err == nil {
		t.Error("expected error for oversized body, got nil")
	}
}

func TestRemoteIP_LoopbackNotTrusted(t *testing.T) {
	// When direct connection is from private IP and X-Real-IP is loopback,
	// it should NOT trust the header and return the direct address.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-Ip", "127.0.0.1")
	ip := remoteIP(req)
	if ip == "127.0.0.1" {
		t.Errorf("remoteIP should not trust loopback X-Real-IP, got %s", ip)
	}
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

func TestRemoteIP_XForwardedForLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1, 10.0.0.2")
	ip := remoteIP(req)
	if ip == "127.0.0.1" {
		t.Errorf("remoteIP should not trust loopback X-Forwarded-For, got %s", ip)
	}
}

func TestMaxJSONBodyConst(t *testing.T) {
	if maxJSONBody != 1<<20 {
		t.Errorf("maxJSONBody = %d, want %d", maxJSONBody, 1<<20)
	}
}

// ── Static source analysis tests ──────────────────────────────────────────────
// These tests use os.ReadFile + strings.Contains to verify security properties
// of source files, because the packages have complex dependencies that prevent
// unit testing without a full server setup.

func TestSourceSecurity_HandleKeycenter_PromoteRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleKeycenterPromoteToVault)") {
		t.Error("handleKeycenterPromoteToVault must be wrapped with requireAdminAuth")
	}
	if !strings.Contains(content, "requireUnlocked(s.requireAdminAuth(s.handleKeycenterPromoteToVault))") {
		t.Error("handleKeycenterPromoteToVault must also be wrapped with requireUnlocked")
	}
}

func TestSourceSecurity_HandleKeycenter_TempRefCreateRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleKeycenterCreateTempRef)") {
		t.Error("handleKeycenterCreateTempRef must be wrapped with requireAdminAuth")
	}
	if !strings.Contains(content, `"POST /api/keycenter/temp-refs"`) {
		t.Error("POST /api/keycenter/temp-refs route must be registered")
	}
}

func TestSourceSecurity_HandleKeycenter_TempRefListRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleKeycenterTempRefs)") {
		t.Error("handleKeycenterTempRefs (list) must be wrapped with requireAdminAuth")
	}
}

func TestSourceSecurity_HandleKeycenter_RevealRefRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleKeycenterRevealRef)") {
		t.Error("handleKeycenterRevealRef must be wrapped with requireAdminAuth")
	}
}

func TestSourceSecurity_RegistrationTokens_CreateRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleCreateRegistrationToken)") {
		t.Error("handleCreateRegistrationToken must be wrapped with requireAdminAuth")
	}
	if !strings.Contains(content, `"POST /api/admin/registration-tokens"`) {
		t.Error("POST /api/admin/registration-tokens route must be registered")
	}
}

func TestSourceSecurity_RegistrationTokens_ListRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleListRegistrationTokens)") {
		t.Error("handleListRegistrationTokens must be wrapped with requireAdminAuth")
	}
	if !strings.Contains(content, `"GET /api/admin/registration-tokens"`) {
		t.Error("GET /api/admin/registration-tokens route must be registered")
	}
}

func TestSourceSecurity_RegistrationTokens_RevokeRequiresAdminAuth(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireAdminAuth(s.handleRevokeRegistrationToken)") {
		t.Error("handleRevokeRegistrationToken must be wrapped with requireAdminAuth")
	}
	if !strings.Contains(content, `"DELETE /api/admin/registration-tokens/{token_id}"`) {
		t.Error("DELETE /api/admin/registration-tokens/{token_id} route must be registered")
	}
}

func TestSourceSecurity_NameValidation_DelegatesToHttputil(t *testing.T) {
	src, err := os.ReadFile("name_validation.go")
	if err != nil {
		t.Fatalf("failed to read name_validation.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "httputil.IsValidResourceName") {
		t.Error("isValidResourceName must delegate to httputil.IsValidResourceName")
	}
}

func TestSourceSecurity_GCTempRefs_DeletesExpiredTokens(t *testing.T) {
	src, err := os.ReadFile("gc_temp_refs.go")
	if err != nil {
		t.Fatalf("failed to read gc_temp_refs.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "DeleteExpiredTempRefs") {
		t.Error("StartTempRefGC must call DeleteExpiredTempRefs")
	}
	if !strings.Contains(content, "DeleteExpiredRegistrationTokens") {
		t.Error("StartTempRefGC must call DeleteExpiredRegistrationTokens")
	}
	if !strings.Contains(content, "AutoArchiveStaleAgents") {
		t.Error("StartTempRefGC must call AutoArchiveStaleAgents")
	}
}

func TestSourceSecurity_GCTempRefs_StopChannel(t *testing.T) {
	src, err := os.ReadFile("gc_temp_refs.go")
	if err != nil {
		t.Fatalf("failed to read gc_temp_refs.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "stop <-chan struct{}") {
		t.Error("StartTempRefGC must accept a stop channel for graceful shutdown")
	}
	if !strings.Contains(content, "case <-stop:") {
		t.Error("StartTempRefGC must listen on stop channel")
	}
}

func TestSourceSecurity_TempEncrypt_RequiresTrustedIPAndUnlocked(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireTrustedIP(s.requireUnlocked(s.handleTempEncrypt))") {
		t.Error("handleTempEncrypt must be wrapped with requireTrustedIP and requireUnlocked")
	}
	if !strings.Contains(content, `"POST /api/encrypt"`) {
		t.Error("POST /api/encrypt route must be registered")
	}
}

func TestSourceSecurity_TempEncrypt_ChecksPlaintext(t *testing.T) {
	src, err := os.ReadFile("handlers_temp_encrypt.go")
	if err != nil {
		t.Fatalf("failed to read handlers_temp_encrypt.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, `req.Plaintext == ""`) {
		t.Error("handleTempEncrypt must validate plaintext is not empty")
	}
	if !strings.Contains(content, "FindActiveTempRefByHash") {
		t.Error("handleTempEncrypt must check for existing temp ref by hash (dedup)")
	}
}

func TestSourceSecurity_AuditRoute_RequiresReadyMiddleware(t *testing.T) {
	src, err := os.ReadFile("hkm/handler.go")
	if err != nil {
		t.Fatalf("failed to read hkm/handler.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, `ready(h.handleAuditEventsList)`) {
		t.Error("handleAuditEventsList must be wrapped with ready middleware")
	}
	if !strings.Contains(content, `"GET /api/catalog/audit"`) {
		t.Error("GET /api/catalog/audit route must be registered")
	}
}

func TestSourceSecurity_Audit_SavesEventWithUUID(t *testing.T) {
	src, err := os.ReadFile("audit.go")
	if err != nil {
		t.Fatalf("failed to read audit.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "crypto.GenerateUUID()") {
		t.Error("saveAuditEvent must generate a unique UUID for each event")
	}
	if !strings.Contains(content, "s.db.SaveAuditEvent") {
		t.Error("saveAuditEvent must persist the event to DB")
	}
}

func TestSourceSecurity_Audit_ActorIDExtractsIP(t *testing.T) {
	src, err := os.ReadFile("audit.go")
	if err != nil {
		t.Fatalf("failed to read audit.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "net.SplitHostPort") {
		t.Error("normalizeActorRemoteAddr must use net.SplitHostPort for correct IP extraction")
	}
	if !strings.Contains(content, "func actorIDForRequest(r *http.Request) string") {
		t.Error("actorIDForRequest must be defined")
	}
}

func TestSourceSecurity_AdminLogin_NoAdminAuthMiddleware(t *testing.T) {
	// Login endpoint must NOT require adminAuth (chicken-and-egg)
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	// The login line should NOT have requireAdminAuth wrapping
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "handleAdminLogin") {
			if strings.Contains(line, "requireAdminAuth") {
				t.Error("handleAdminLogin must NOT be wrapped with requireAdminAuth")
			}
			break
		}
	}
}

func TestSourceSecurity_ValidateToken_RequiresTrustedIP(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatalf("failed to read handlers.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, "requireTrustedIP(s.requireUnlocked(s.handleValidateRegistrationToken))") {
		t.Error("handleValidateRegistrationToken must be wrapped with requireTrustedIP + requireUnlocked")
	}
}
