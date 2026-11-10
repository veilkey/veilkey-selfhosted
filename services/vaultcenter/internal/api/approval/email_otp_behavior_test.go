package approval

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"veilkey-vaultcenter/internal/db"
)

func newEmailOTPTestHandler(t *testing.T) (*Handler, string, string) {
	t.Helper()
	t.Setenv("VEILKEY_EMAIL_OTP_EXPIRY", "5m")
	t.Setenv("VEILKEY_EMAIL_OTP_MAX_ATTEMPTS", "3")
	t.Setenv("VEILKEY_EMAIL_OTP_SEND_COOLDOWN", "1ns")
	t.Setenv("VEILKEY_EMAIL_OTP_MAX_RESENDS", "2")
	t.Setenv("VEILKEY_SMTP_HOST", "")
	t.Setenv("VEILKEY_SENDMAIL", "/usr/bin/true")
	database, err := db.New(filepath.Join(t.TempDir(), "otp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	code := fmt.Sprintf("%06d", secureRandInt(1000000))
	expires := time.Now().UTC().Add(time.Minute)
	challenge := &db.EmailOTPChallenge{
		Token: "otp-test-challenge", Email: "otp@example.invalid", Status: "pending",
		CodeHash: hashEmailOTPCode(code), CodeExpiresAt: &expires,
	}
	if err := database.SaveEmailOTPChallenge(challenge); err != nil {
		t.Fatal(err)
	}
	return NewHandler(database, nil, nil), challenge.Token, code
}

func submitEmailOTP(h *Handler, token, action, code string) *httptest.ResponseRecorder {
	form := url.Values{"token": {token}, "action": {action}, "code": {code}}
	req := httptest.NewRequest(http.MethodPost, "/ui/approvals/email-otp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	h.handleSubmitEmailOTP(res, req)
	return res
}

func TestEmailOTPWrongAttemptBound(t *testing.T) {
	h, token, code := newEmailOTPTestHandler(t)
	for attempt := 1; attempt <= 3; attempt++ {
		want := http.StatusForbidden
		if attempt == 3 {
			want = http.StatusTooManyRequests
		}
		if got := submitEmailOTP(h, token, "verify", "incorrect").Code; got != want {
			t.Fatalf("attempt %d: status %d, want %d", attempt, got, want)
		}
	}
	if got := submitEmailOTP(h, token, "verify", code).Code; got != http.StatusTooManyRequests {
		t.Fatalf("locked challenge accepted valid code: status %d", got)
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("locked challenge allowed resend: status %d", got)
	}
}

func TestEmailOTPResendBound(t *testing.T) {
	h, token, _ := newEmailOTPTestHandler(t)
	for send := 1; send <= 3; send++ {
		if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusOK {
			t.Fatalf("send %d: status %d", send, got)
		}
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("resend limit: status %d, want 429", got)
	}
}

func TestEmailOTPResendCooldown(t *testing.T) {
	h, token, _ := newEmailOTPTestHandler(t)
	t.Setenv("VEILKEY_EMAIL_OTP_SEND_COOLDOWN", "1h")
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusOK {
		t.Fatalf("first send: status %d", got)
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("cooldown: status %d, want 429", got)
	}
}

func TestEmailOTPResendPreservesFailedAttempts(t *testing.T) {
	h, token, _ := newEmailOTPTestHandler(t)
	for attempt := 0; attempt < 2; attempt++ {
		if got := submitEmailOTP(h, token, "verify", "incorrect").Code; got != http.StatusForbidden {
			t.Fatalf("wrong code: status %d", got)
		}
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusOK {
		t.Fatalf("send code: status %d", got)
	}
	if got := submitEmailOTP(h, token, "verify", "incorrect").Code; got != http.StatusTooManyRequests {
		t.Fatalf("attempts reset by resend: status %d", got)
	}
}

func TestEmailOTPFailedDeliveryConsumesSend(t *testing.T) {
	h, token, _ := newEmailOTPTestHandler(t)
	t.Setenv("VEILKEY_SENDMAIL", filepath.Join(t.TempDir(), "missing-sendmail"))
	t.Setenv("VEILKEY_EMAIL_OTP_MAX_RESENDS", "0")
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusBadGateway {
		t.Fatalf("failed delivery: status %d", got)
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("failed delivery replenished send budget: status %d", got)
	}
}

func TestEmailOTPInvalidPolicyFailsClosed(t *testing.T) {
	for _, setting := range []struct{ key, value string }{
		{"VEILKEY_EMAIL_OTP_MAX_ATTEMPTS", ""},
		{"VEILKEY_EMAIL_OTP_MAX_ATTEMPTS", "0"},
		{"VEILKEY_EMAIL_OTP_MAX_RESENDS", "-1"},
		{"VEILKEY_EMAIL_OTP_SEND_COOLDOWN", "0s"},
		{"VEILKEY_EMAIL_OTP_EXPIRY", "invalid"},
	} {
		t.Run(setting.key+"="+setting.value, func(t *testing.T) {
			h, token, code := newEmailOTPTestHandler(t)
			t.Setenv(setting.key, setting.value)
			for _, action := range []string{"send-code", "verify"} {
				if got := submitEmailOTP(h, token, action, code).Code; got != http.StatusServiceUnavailable {
					t.Fatalf("invalid policy allowed %s: status %d", action, got)
				}
			}
		})
	}
}

func TestEmailOTPExpiredCodeRejected(t *testing.T) {
	h, token, code := newEmailOTPTestHandler(t)
	challenge, err := h.db.GetEmailOTPChallenge(token)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	challenge.CodeExpiresAt = &expired
	if err := h.db.SaveEmailOTPChallenge(challenge); err != nil {
		t.Fatal(err)
	}
	if got := submitEmailOTP(h, token, "verify", code).Code; got != http.StatusForbidden {
		t.Fatalf("expired code: status %d", got)
	}
}

func TestEmailOTPLegitimateVerification(t *testing.T) {
	h, token, code := newEmailOTPTestHandler(t)
	if got := submitEmailOTP(h, token, "verify", "incorrect").Code; got != http.StatusForbidden {
		t.Fatalf("wrong code: status %d", got)
	}
	if got := submitEmailOTP(h, token, "verify", code).Code; got != http.StatusOK {
		t.Fatalf("valid code: status %d", got)
	}
	challenge, err := h.db.GetEmailOTPChallenge(token)
	if err != nil || challenge.Status != "verified" || challenge.UsedAt == nil {
		t.Fatalf("verification not persisted: challenge %+v, error %v", challenge, err)
	}
	if got := submitEmailOTP(h, token, "send-code", "").Code; got != http.StatusForbidden {
		t.Fatalf("verified challenge allowed resend: status %d", got)
	}
	if got := submitEmailOTP(h, token, "verify", code).Code; got != http.StatusForbidden {
		t.Fatalf("verified code replay: status %d", got)
	}
}
