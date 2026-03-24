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
