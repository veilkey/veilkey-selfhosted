package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
	"veilkey-vaultcenter/internal/mailer"

	vcrypto "github.com/veilkey/veilkey-go-package/crypto"
)

type emailOTPRequest struct {
	Email   string `json:"email"`
	Reason  string `json:"reason"`
	BaseURL string `json:"base_url"`
}

func (h *Handler) handleCreateEmailOTPChallenge(w http.ResponseWriter, r *http.Request) {
	var req emailOTPRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		respondErr(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.TrimSpace(req.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid email format")
		return
	}
	token := vcrypto.GenerateUUID()
	challenge := &db.EmailOTPChallenge{
		Token:  token,
		Email:  strings.TrimSpace(req.Email),
		Reason: strings.TrimSpace(req.Reason),
		Status: "pending",
	}
	if err := h.db.SaveEmailOTPChallenge(challenge); err != nil {
		log.Printf("email-otp: failed to save challenge: %v", err)
		respondErr(w, http.StatusBadRequest, "failed to create challenge")
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = httputil.RequestBaseURL(r)
	}
	link := baseURL + "/ui/approvals/email-otp?token=" + token
	body := strings.Join([]string{
		fmt.Sprintf("VeilKey verification link for %s", httputil.RequestBaseURL(r)),
		"",
		"Purpose: approve the current VeilKey sensitive action",
		fmt.Sprintf("Action: %s", defaultEmailOTPReason(challenge.Reason)),
		"Scope: this approval is for the current pending VeilKey action only",
		fmt.Sprintf("Approve URL: %s", link),
		"How to approve:",
		"1. Open the URL above",
		`2. Click "Send code by email"`,
		"3. Receive the 6-digit code and paste it into the web page",
		"4. Re-run the original VeilKey command",
	}, "\n")
	if err := mailer.Send(challenge.Email, "VeilKey verification code", body); err != nil {
		log.Printf("email-otp: failed to send email: %v", err)
		respondErr(w, http.StatusBadGateway, "failed to send email")
		return
	}
	respond(w, http.StatusCreated, map[string]any{
		"token": token,
		"link":  link,
	})
}

func (h *Handler) handleEmailOTPState(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := h.db.GetEmailOTPChallenge(token)
	if err != nil {
		respondErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"token":  challenge.Token,
		"email":  challenge.Email,
		"reason": challenge.Reason,
		"status": challenge.Status,
	})
}

func (h *Handler) handleEmailOTPPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := h.db.GetEmailOTPChallenge(token)
	if err != nil {
		respondErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	if challenge.Status == "verified" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, emailOTPSuccessHTML)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, emailOTPHTML, challenge.Email, defaultEmailOTPReason(challenge.Reason), token)
}

func (h *Handler) handleSubmitEmailOTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16) // 64KB
	if err := r.ParseForm(); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid form body")
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	action := strings.TrimSpace(r.FormValue("action"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := h.db.GetEmailOTPChallenge(token)
	if err != nil {
		respondErr(w, http.StatusNotFound, "challenge not found")
		return
	}
	policy, err := loadEmailOTPPolicy()
	if err != nil {
		respondErr(w, http.StatusServiceUnavailable, "email otp policy is not configured")
		return
	}
	switch action {
	case "send-code":
		code := fmt.Sprintf("%06d", secureRandInt(1000000))
		expiresAt := time.Now().UTC().Add(policy.expiry)
		if err := h.db.IssueEmailOTPCode(token, hashEmailOTPCode(code), expiresAt, policy.maxAttempts, policy.maxResends, policy.cooldown); err != nil {
			respondEmailOTPError(w, err)
			return
		}
		body := fmt.Sprintf("VeilKey one-time code\n\nCode: %s\nExpires in: %d seconds\n", code, int(policy.expiry.Seconds()))
		if err := mailer.Send(challenge.Email, "VeilKey one-time code", body); err != nil {
			log.Printf("email-otp: failed to send code email: %v", err)
			respondErr(w, http.StatusBadGateway, "failed to send code")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, emailOTPCodeHTML, challenge.Email, defaultEmailOTPReason(challenge.Reason), token)
	case "verify":
		code := strings.TrimSpace(r.FormValue("code"))
		if err := h.db.VerifyEmailOTPChallenge(token, policy.maxAttempts, func(current *db.EmailOTPChallenge) bool {
			return validateEmailOTPChallenge(current, code)
		}); err != nil {
			respondEmailOTPError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, emailOTPSuccessHTML)
	default:
		respondErr(w, http.StatusBadRequest, "unsupported action")
	}
}

type emailOTPPolicy struct {
	maxAttempts int
	maxResends  int
	cooldown    time.Duration
	expiry      time.Duration
}

func loadEmailOTPPolicy() (emailOTPPolicy, error) {
	var policy emailOTPPolicy
	var err error
	policy.maxAttempts, err = strconv.Atoi(strings.TrimSpace(os.Getenv("VEILKEY_EMAIL_OTP_MAX_ATTEMPTS")))
	if err != nil || policy.maxAttempts <= 0 {
		return policy, fmt.Errorf("VEILKEY_EMAIL_OTP_MAX_ATTEMPTS must be a positive integer")
	}
	policy.maxResends, err = strconv.Atoi(strings.TrimSpace(os.Getenv("VEILKEY_EMAIL_OTP_MAX_RESENDS")))
	if err != nil || policy.maxResends < 0 {
		return policy, fmt.Errorf("VEILKEY_EMAIL_OTP_MAX_RESENDS must be a nonnegative integer")
	}
	policy.cooldown, err = time.ParseDuration(strings.TrimSpace(os.Getenv("VEILKEY_EMAIL_OTP_SEND_COOLDOWN")))
	if err != nil || policy.cooldown <= 0 {
		return policy, fmt.Errorf("VEILKEY_EMAIL_OTP_SEND_COOLDOWN must be a positive duration")
	}
	policy.expiry, err = time.ParseDuration(strings.TrimSpace(os.Getenv("VEILKEY_EMAIL_OTP_EXPIRY")))
	if err != nil || policy.expiry <= 0 {
		return policy, fmt.Errorf("VEILKEY_EMAIL_OTP_EXPIRY must be a positive duration")
	}
	return policy, nil
}

func respondEmailOTPError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrEmailOTPLimited):
		respondErr(w, http.StatusTooManyRequests, "email otp challenge limit reached")
	case errors.Is(err, db.ErrEmailOTPInvalid):
		respondErr(w, http.StatusForbidden, "code is invalid or expired")
	default:
		log.Printf("email-otp: challenge update failed")
		respondErr(w, http.StatusInternalServerError, "email otp operation failed")
	}
}

func validateEmailOTPChallenge(challenge *db.EmailOTPChallenge, code string) bool {
	if challenge == nil || strings.TrimSpace(code) == "" || challenge.CodeExpiresAt == nil || time.Now().UTC().After(*challenge.CodeExpiresAt) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashEmailOTPCode(code)), []byte(challenge.CodeHash)) == 1
}

func hashEmailOTPCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// secureRandInt returns a cryptographically random int in [0, max) with no modular bias.
func secureRandInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

func defaultEmailOTPReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "manual send"
	}
	return reason
}

const emailOTPHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Email OTP</title></head>
<body><div style="max-width:640px;margin:8vh auto;padding:24px;font-family:sans-serif">
<h1>Email OTP Approval</h1>
<p>Target email: <strong>%s</strong></p>
<p>Purpose: %s</p>
<form method="post" action="/ui/approvals/email-otp">
<input type="hidden" name="token" value="%s">
<input type="hidden" name="action" value="send-code">
<button type="submit">Send code by email</button>
</form>
</div></body></html>`

const emailOTPCodeHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Email OTP</title></head>
<body><div style="max-width:640px;margin:8vh auto;padding:24px;font-family:sans-serif">
<h1>Email OTP Approval</h1>
<p>Target email: <strong>%s</strong></p>
<p>Purpose: %s</p>
<p>A 6-digit code was sent by email.</p>
<form method="post" action="/ui/approvals/email-otp">
<input type="hidden" name="token" value="%s">
<input type="hidden" name="action" value="verify">
<input type="text" name="code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" placeholder="123456" autofocus>
<button type="submit">Verify code</button>
</form>
</div></body></html>`

const emailOTPSuccessHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Email OTP</title></head>
<body><div style="max-width:640px;margin:8vh auto;padding:24px;font-family:sans-serif"><h1>Approval complete</h1><p>The approval is complete. Re-run the original VeilKey command.</p></div></body></html>`
