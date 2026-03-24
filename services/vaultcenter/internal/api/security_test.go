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

// ── Source analysis: bulk/templates.go — allowlist enforcement ────────────────

func TestSource_BulkApplyTemplates_AllowedFormatsWhitelist(t *testing.T) {
	src, err := os.ReadFile("bulk/files.go")
	if err != nil {
		t.Fatalf("failed to read bulk/files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "allowedBulkApplyFormatsFile") {
		t.Error("bulk apply templates must define an allowlist of permitted formats")
	}
	for _, format := range []string{`"env"`, `"json"`, `"json_merge"`, `"raw"`} {
		if !strings.Contains(content, format) {
			t.Errorf("allowed formats must include: %s", format)
		}
	}
}

func TestSource_BulkApplyTemplates_ValidatesFormat(t *testing.T) {
	src, err := os.ReadFile("bulk/files.go")
	if err != nil {
		t.Fatalf("failed to read bulk/files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `allowedBulkApplyFormatsFile[format]`) {
		t.Error("template normalization must validate format against allowlist")
	}
}

func TestSource_BulkApplyTemplates_SensitiveValueMasking(t *testing.T) {
	src, err := os.ReadFile("bulk/templates.go")
	if err != nil {
		t.Fatalf("failed to read bulk/templates.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func isSensitiveBulkApplyValue(") {
		t.Error("templates must define isSensitiveBulkApplyValue for masking")
	}
	if !strings.Contains(content, "func maskBulkApplyValue(") {
		t.Error("templates must define maskBulkApplyValue for preview masking")
	}
	for _, keyword := range []string{"PASSWORD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE"} {
		if !strings.Contains(content, keyword) {
			t.Errorf("isSensitiveBulkApplyValue must check for keyword: %s", keyword)
		}
	}
}

func TestSource_BulkApplyTemplates_WriteRoutesRequireTrustedIP(t *testing.T) {
	src, err := os.ReadFile("bulk/handler.go")
	if err != nil {
		t.Fatalf("failed to read bulk/handler.go: %v", err)
	}
	content := string(src)

	writeRoutes := []string{
		"POST /api/vaults/{vault}/bulk-apply/templates",
		"PUT /api/vaults/{vault}/bulk-apply/templates/{name}",
		"DELETE /api/vaults/{vault}/bulk-apply/templates/{name}",
	}
	for _, route := range writeRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "requireTrustedIP") {
					t.Errorf("write route %s must be wrapped with requireTrustedIP", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("write route not registered: %s", route)
		}
	}
}

// ── Source analysis: models.go — ApprovalTokenChallenge lifecycle fields ──────

func TestSource_ApprovalTokenChallenge_LifecycleFields(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "type ApprovalTokenChallenge struct") {
		t.Fatal("ApprovalTokenChallenge model must exist")
	}

	fields := []string{
		"Token",
		"Kind",
		"Status",
		"CreatedAt",
		"UpdatedAt",
		"UsedAt",
	}
	for _, field := range fields {
		if !strings.Contains(content, field) {
			t.Errorf("ApprovalTokenChallenge must have lifecycle field: %s", field)
		}
	}
}

func TestSource_ApprovalTokenChallenge_HasPromptFields(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	for _, field := range []string{"Title", "Prompt", "InputLabel", "SubmitLabel"} {
		if !strings.Contains(content, field) {
			t.Errorf("ApprovalTokenChallenge must have prompt field: %s", field)
		}
	}
}

func TestSource_ApprovalTokenChallenge_HasEncryptedPayload(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "Ciphertext") || !strings.Contains(content, "Nonce") {
		t.Error("ApprovalTokenChallenge must have Ciphertext and Nonce fields for encrypted payload")
	}
}

func TestSource_ApprovalTokenChallenge_DefaultStatusPending(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `default:pending`) {
		t.Error("ApprovalTokenChallenge.Status must default to 'pending'")
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

func TestAgentModelHasSaltField(t *testing.T) {
	s, _ := os.ReadFile("../db/models.go")
	if !strings.Contains(string(s), `gorm:"column:salt"`) {
		t.Error("Agent model must have Salt field")
	}
}

func TestHeartbeatAcceptsSalt(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_heartbeat.go")
	c := string(s)
	if !strings.Contains(c, `json:"salt`) {
		t.Error("heartbeat req must have Salt field")
	}
	if !strings.Contains(c, "req.Salt") {
		t.Error("upsertPayload must include req.Salt")
	}
}

func TestUnlockKeyResponseIncludesSalt(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_unlock_key.go")
	if !strings.Contains(string(s), `"salt"`) {
		t.Error("unlock-key response must include salt")
	}
}

func TestUpsertAgentHasSaltParam(t *testing.T) {
	s, _ := os.ReadFile("../db/db_agent.go")
	b := extractFn(string(s), "func (d *DB) UpsertAgent(")
	if !strings.Contains(b, "salt string") {
		t.Error("UpsertAgent needs salt param")
	}
}

// ══════════════════════════════════════════════════════════════════
// Salt-on-chain: design principle verification
// VC must be the authoritative source for salt.
// ══════════════════════════════════════════════════════════════════

// --- Chain TX includes salt ---

func TestChainUpsertPayloadHasSalt(t *testing.T) {
	// veilkey-chain tx.go must have Salt in UpsertAgentPayload
	// We verify via the heartbeat code that forwards req.Salt to chain TX
	s, _ := os.ReadFile("hkm/hkm_agent_heartbeat.go")
	c := string(s)
	// req struct must accept salt
	if !strings.Contains(c, `Salt`) || !strings.Contains(c, `json:"salt`) {
		t.Error("heartbeat req must accept salt from LV")
	}
	// upsertPayload must forward salt to chain
	if !strings.Contains(c, "Salt:") && !strings.Contains(c, "req.Salt") {
		t.Error("upsertPayload must forward req.Salt to chain TX")
	}
}

// --- Agent DB stores salt ---

func TestAgentDBStoresSaltOnCreate(t *testing.T) {
	s, _ := os.ReadFile("../db/db_agent.go")
	b := extractFn(string(s), "func (d *DB) UpsertAgent(")
	// Must have Salt in Create block
	createIdx := strings.Index(b, "d.conn.Create")
	if createIdx < 0 {
		t.Fatal("UpsertAgent must have Create call")
	}
	beforeCreate := b[:createIdx]
	if !strings.Contains(beforeCreate, "Salt:") {
		t.Error("UpsertAgent must set Salt on create (new agent)")
	}
}

func TestAgentDBUpdatesSaltConditionally(t *testing.T) {
	s, _ := os.ReadFile("../db/db_agent.go")
	b := extractFn(string(s), "func (d *DB) UpsertAgent(")
	// Must only update salt when not empty (preserve existing)
	if !strings.Contains(b, `if salt != ""`) {
		t.Error("UpsertAgent must conditionally update salt (only when non-empty)")
	}
}

// --- Salt not exposed in JSON API ---

func TestAgentSaltNotInJSON(t *testing.T) {
	s, _ := os.ReadFile("../db/models.go")
	// Find the Salt field line
	for _, line := range strings.Split(string(s), "\n") {
		if strings.Contains(line, "Salt") && strings.Contains(line, "gorm:") {
			if !strings.Contains(line, `json:"-"`) {
				t.Error("Agent.Salt must have json:\"-\" to prevent exposure in API responses")
			}
			return
		}
	}
	t.Error("Agent.Salt field not found in models.go")
}

// --- unlock-key response includes salt ---

func TestUnlockKeyResponseSaltField(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_unlock_key.go")
	c := string(s)
	if !strings.Contains(c, `"salt"`) || !strings.Contains(c, "agent.Salt") {
		t.Error("unlock-key response must include agent.Salt")
	}
}

// --- Heartbeat salt is omitempty (backward compat) ---

func TestHeartbeatSaltOmitempty(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_heartbeat.go")
	for _, line := range strings.Split(string(s), "\n") {
		if strings.Contains(line, "Salt") && strings.Contains(line, "json:") {
			if !strings.Contains(line, "omitempty") {
				t.Error("heartbeat Salt must be omitempty for backward compatibility")
			}
			return
		}
	}
	t.Error("Salt field not found in heartbeat req struct")
}

// --- ChainStoreAdapter forwards salt ---

func TestChainStoreAdapterForwardsSalt(t *testing.T) {
	s, _ := os.ReadFile("../db/chain_store.go")
	b := extractFn(string(s), "func (a *ChainStoreAdapter) UpsertAgent(")
	if !strings.Contains(b, "salt string") && !strings.Contains(b, "salt)") {
		t.Error("ChainStoreAdapter.UpsertAgent must forward salt to DB")
	}
}

// ── Bug 1: VC Unlock must double-check locked state under write lock ─────────

func TestUnlockHasDoubleCheckUnderWriteLock(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (s *Server) Unlock(kek []byte) error {")
	if fnBody == "" {
		t.Fatal("Unlock method must exist")
	}

	// Must acquire write lock
	if !strings.Contains(fnBody, "s.kekMu.Lock()") {
		t.Error("Unlock must acquire write lock (s.kekMu.Lock())")
	}

	// Must double-check locked state after acquiring write lock
	lockIdx := strings.Index(fnBody, "s.kekMu.Lock()")
	if lockIdx < 0 {
		t.Fatal("s.kekMu.Lock() not found")
	}
	afterLock := fnBody[lockIdx:]
	if !strings.Contains(afterLock, "!s.locked") {
		t.Error("Unlock must check !s.locked after acquiring write lock (double-check pattern)")
	}

	// Must close database on race (another goroutine unlocked first)
	if !strings.Contains(afterLock, "database.Close()") {
		t.Error("Unlock must close database connection if another goroutine already unlocked")
	}
}

// ══════════════════════════════════════════════════════════════════
// Logic bug regression tests
// ══════════════════════════════════════════════════════════════════

func TestUnlockDoubleCheckUnderWriteLock(t *testing.T) {
	s, _ := os.ReadFile("api.go")
	b := extractFn(string(s), "func (s *Server) Unlock(")
	if !strings.Contains(b, "!s.locked") {
		t.Error("Unlock must double-check locked state under write lock")
	}
	if !strings.Contains(b, "database.Close()") {
		t.Error("Unlock must close leaked DB on race")
	}
}

func TestRevealWindowNotExtendable(t *testing.T) {
	s, _ := os.ReadFile("admin/admin_auth.go")
	b := extractFn(string(s), "func (h *Handler) handleAdminRevealAuthorize(")
	if !strings.Contains(b, "RevealUntil") || !strings.Contains(b, "not extended") {
		t.Error("reveal window must not be extendable when already active")
	}
}

func TestRegression_PromoteRejectsDeletedAgent(t *testing.T) {
	s, _ := os.ReadFile("handle_keycenter.go")
	b := extractFn(string(s), "func (s *Server) handleKeycenterPromoteToVault(")
	if !strings.Contains(b, "DeletedAt") {
		t.Error("promote must reject deleted agents")
	}
}

func TestKeyVersionMismatchHasRetryCap(t *testing.T) {
	s, _ := os.ReadFile("hkm/agent_state_helpers.go")
	if !strings.Contains(string(s), "AgentRetrySchedule") {
		t.Error("retry must have schedule cap")
	}
	b := extractFn(string(s), "func advanceRebindPayload(")
	if !strings.Contains(b, ">= len(") {
		t.Error("must cap retry stage at schedule length")
	}
}

// ══ Error message leakage prevention ════════════════════════════

func TestAgentArchiveNoRawError(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_archive.go")
	c := string(s)
	if strings.Contains(c, "err.Error()") {
		t.Error("agent archive must not expose raw error in HTTP response")
	}
}

func TestAgentUnregisterNoRawError(t *testing.T) {
	s, _ := os.ReadFile("hkm/hkm_agent_unregister.go")
	c := string(s)
	// respondError with err.Error() leaks internal details
	for _, line := range strings.Split(c, "\n") {
		if strings.Contains(line, "respondError") && strings.Contains(line, "err.Error()") {
			t.Error("agent unregister must not expose raw error in HTTP response")
		}
	}
}

func TestRegistrationTokenCreateHasMaxBytes(t *testing.T) {
	s, _ := os.ReadFile("handle_registration_tokens.go")
	b := extractFn(string(s), "func (s *Server) handleCreateRegistrationToken(")
	if !strings.Contains(b, "MaxBytesReader") {
		t.Error("registration token create must have MaxBytesReader")
	}
}

func TestSMTPErrorNoCredentials(t *testing.T) {
	s, _ := os.ReadFile("../mailer/mailer.go")
	c := string(s)
	// Error messages must not wrap internal errors (which may contain credentials)
	for _, line := range strings.Split(c, "\n") {
		if strings.Contains(line, "fmt.Errorf") && strings.Contains(line, "%w") {
			t.Errorf("mailer must not wrap errors (may leak SMTP credentials): %s", strings.TrimSpace(line))
		}
	}
}

// ══ Error sanitization regression ═══════════════════════════════

func TestNoRawErrorsInPluginHandler(t *testing.T) {
	src, _ := os.ReadFile("../../plugin/handler.go")
	if src == nil {
		t.Skip("plugin handler not accessible")
	}
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "respondError") && strings.Contains(line, "err.Error()") {
			t.Errorf("plugin/handler.go:%d leaks raw error", i+1)
		}
	}
}

func TestNoRawErrorsInPasskey(t *testing.T) {
	src, _ := os.ReadFile("admin/passkey.go")
	if src == nil {
		t.Skip("passkey not accessible")
	}
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "respondError") && strings.Contains(line, "err.Error()") {
			t.Errorf("passkey.go:%d leaks raw error", i+1)
		}
	}
}

func TestNoRawErrorsInFunctionRun(t *testing.T) {
	src, _ := os.ReadFile("hkm/hkm_global_function_run.go")
	if src == nil {
		t.Skip("function run not accessible")
	}
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "respondError") && strings.Contains(line, "err.Error()") {
			t.Errorf("function_run.go:%d leaks raw error", i+1)
		}
	}
}
