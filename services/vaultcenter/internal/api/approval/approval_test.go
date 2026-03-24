package approval

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: tokens.go — HTML escaping (XSS prevention) ───────────────

func TestSource_TokenHTML_EscapesOutput(t *testing.T) {
	src, err := os.ReadFile("tokens.go")
	if err != nil {
		t.Fatalf("failed to read tokens.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func escapeApprovalHTML(`) {
		t.Error("escapeApprovalHTML function must exist for XSS prevention")
	}
	// Must escape all five standard HTML entities
	for _, entity := range []string{"&amp;", "&lt;", "&gt;", "&quot;", "&#39;"} {
		if !strings.Contains(content, entity) {
			t.Errorf("escapeApprovalHTML must produce HTML entity: %s", entity)
		}
	}
}

func TestSource_TokenHTML_AllFieldsEscaped(t *testing.T) {
	src, err := os.ReadFile("tokens.go")
	if err != nil {
		t.Fatalf("failed to read tokens.go: %v", err)
	}
	content := string(src)

	// The handleApprovalTokenPage function must escape all dynamic values
	fnBody := extractFn(content, "func (h *Handler) handleApprovalTokenPage(")
	escapeCount := strings.Count(fnBody, "escapeApprovalHTML(")
	if escapeCount < 4 {
		t.Errorf("handleApprovalTokenPage must escape at least 4 fields, found %d calls to escapeApprovalHTML", escapeCount)
	}
}

func TestUnit_EscapeApprovalHTML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`<script>alert("xss")</script>`, `&lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;`},
		{`Hello & goodbye`, `Hello &amp; goodbye`},
		{`it's fine`, `it&#39;s fine`},
		{`plain text`, `plain text`},
	}
	for _, tt := range tests {
		got := escapeApprovalHTML(tt.input)
		if got != tt.want {
			t.Errorf("escapeApprovalHTML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── Source analysis: email_otp.go — OTP uses crypto/rand ──────────────────────

func TestSource_OTPCodeUsesCryptoRand(t *testing.T) {
	src, err := os.ReadFile("email_otp.go")
	if err != nil {
		t.Fatalf("failed to read email_otp.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/rand"`) {
		t.Error("email_otp.go must import crypto/rand")
	}
	if !strings.Contains(content, `func secureRandInt(`) {
		t.Error("secureRandInt function must exist for unbiased random generation")
	}
	if !strings.Contains(content, `rand.Int(rand.Reader`) {
		t.Error("OTP code generation must use crypto/rand.Int for unbiased randomness")
	}
}

func TestSource_OTPHasBodySizeLimit(t *testing.T) {
	src, err := os.ReadFile("email_otp.go")
	if err != nil {
		t.Fatalf("failed to read email_otp.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `MaxBytesReader`) {
		t.Error("email OTP submit handler must use MaxBytesReader to limit body size")
	}
}

func TestSource_OTPUsesConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("email_otp.go")
	if err != nil {
		t.Fatalf("failed to read email_otp.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/subtle"`) {
		t.Error("email_otp.go must import crypto/subtle")
	}
	if !strings.Contains(content, `subtle.ConstantTimeCompare`) {
		t.Error("OTP verification must use subtle.ConstantTimeCompare")
	}
}

func TestSource_OTPCodeHashedWithSHA256(t *testing.T) {
	src, err := os.ReadFile("email_otp.go")
	if err != nil {
		t.Fatalf("failed to read email_otp.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func hashEmailOTPCode(`) {
		t.Error("hashEmailOTPCode function must exist")
	}
	fnBody := extractFn(content, "func hashEmailOTPCode(")
	if !strings.Contains(fnBody, "sha256.Sum256") {
		t.Error("OTP code hash must use SHA256")
	}
}

// ── Source analysis: secret_input.go — body size limit ────────────────────────

func TestSource_SecretInputHasBodySizeLimit(t *testing.T) {
	src, err := os.ReadFile("secret_input.go")
	if err != nil {
		t.Fatalf("failed to read secret_input.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `MaxBytesReader`) {
		t.Error("secret input submit handler must use MaxBytesReader to limit body size")
	}
}

func TestSource_SecretInputRequiresValueAndConfirm(t *testing.T) {
	src, err := os.ReadFile("secret_input.go")
	if err != nil {
		t.Fatalf("failed to read secret_input.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `req.Value != req.Confirm`) {
		t.Error("secret input must verify value and confirm match")
	}
}

func TestSource_SecretInputStoresViaAgentEndpoint(t *testing.T) {
	src, err := os.ReadFile("secret_input.go")
	if err != nil {
		t.Fatalf("failed to read secret_input.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func (h *Handler) storeSecretViaAgentEndpoint(`) {
		t.Error("storeSecretViaAgentEndpoint function must exist")
	}
	// Must validate endpoint contains /api/agents/
	if !strings.Contains(content, `/api/agents/`) {
		t.Error("storeSecretViaAgentEndpoint must validate endpoint contains /api/agents/")
	}
}

// ── Source analysis: handler.go — route registration ──────────────────────────

func TestSource_Handler_RequireTrustedIPOnWriteRoutes(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	trustedRoutes := []string{
		"POST /api/approvals/email-otp/request",
		"POST /api/approvals/secret-input/request",
	}
	for _, route := range trustedRoutes {
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
			t.Errorf("trusted route not registered: %s", route)
		}
	}
}

func TestSource_Handler_ApprovalTokenRoutesExist(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	routes := []string{
		"GET /approve/t/{token}",
		"POST /approve/t/{token}",
		"GET /ui/approvals/email-otp",
		"POST /ui/approvals/email-otp",
		"GET /ui/approvals/secret-input",
		"POST /ui/approvals/secret-input",
	}
	for _, route := range routes {
		if !strings.Contains(content, route) {
			t.Errorf("approval route not registered: %s", route)
		}
	}
}

// ── Source analysis: tokens.go — challenge status lifecycle ───────────────────

func TestSource_ChallengeStatusLifecycle(t *testing.T) {
	src, err := os.ReadFile("tokens.go")
	if err != nil {
		t.Fatalf("failed to read tokens.go: %v", err)
	}
	content := string(src)

	// Status transitions: pending -> submitted (via CompleteApprovalTokenChallenge)
	if !strings.Contains(content, `"submitted"`) {
		t.Error("tokens.go must reference 'submitted' status")
	}
	if !strings.Contains(content, `challenge.Status == "submitted"`) {
		t.Error("tokens.go must check for already-submitted challenges")
	}
	if !strings.Contains(content, `CompleteApprovalTokenChallenge`) {
		t.Error("tokens.go must call CompleteApprovalTokenChallenge to transition status")
	}
}

// ── Source analysis: email_otp.go — email template fields ─────────────────────

func TestSource_EmailOTPTemplateHasRequiredFields(t *testing.T) {
	src, err := os.ReadFile("email_otp.go")
	if err != nil {
		t.Fatalf("failed to read email_otp.go: %v", err)
	}
	content := string(src)

	requiredFields := []string{
		"Target email:",
		"Send code by email",
		"6-digit code",
		"Verify code",
	}
	for _, field := range requiredFields {
		if !strings.Contains(content, field) {
			t.Errorf("email OTP template must include: %s", field)
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
