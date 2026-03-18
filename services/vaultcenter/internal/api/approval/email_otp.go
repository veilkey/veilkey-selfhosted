package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	vcrypto "veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
	"veilkey-vaultcenter/internal/mailer"
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
	if !strings.Contains(email, "@") || !strings.Contains(email[strings.Index(email, "@"):], ".") {
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
		respondErr(w, http.StatusBadRequest, err.Error())
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
		respondErr(w, http.StatusBadGateway, err.Error())
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
		respondErr(w, http.StatusNotFound, err.Error())
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
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	if challenge.Status == "verified" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, emailOTPSuccessHTML)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, emailOTPHTML, challenge.Email, defaultEmailOTPReason(challenge.Reason), token)
}

func (h *Handler) handleSubmitEmailOTP(w http.ResponseWriter, r *http.Request) {
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
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	switch action {
	case "send-code":
		code := fmt.Sprintf("%06d", rand.IntN(1000000))
		expiresAt := time.Now().UTC().Add(5 * time.Minute)
		if _, err := h.db.UpdateEmailOTPCode(token, hashEmailOTPCode(code), expiresAt); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		body := fmt.Sprintf("VeilKey one-time code\n\nCode: %s\nExpires in: 300 seconds\n", code)
		if err := mailer.Send(challenge.Email, "VeilKey one-time code", body); err != nil {
			respondErr(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, emailOTPCodeHTML, challenge.Email, defaultEmailOTPReason(challenge.Reason), token)
	case "verify":
		code := strings.TrimSpace(r.FormValue("code"))
		if !validateEmailOTPChallenge(challenge, code) {
			respondErr(w, http.StatusForbidden, "code is invalid or expired")
			return
		}
		if _, err := h.db.MarkEmailOTPVerified(token); err != nil {
			respondErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, emailOTPSuccessHTML)
	default:
		respondErr(w, http.StatusBadRequest, "unsupported action")
	}
}

func validateEmailOTPChallenge(challenge *db.EmailOTPChallenge, code string) bool {
	if challenge == nil || strings.TrimSpace(code) == "" || challenge.CodeExpiresAt == nil || time.Now().UTC().After(*challenge.CodeExpiresAt) {
		return false
	}
	return hashEmailOTPCode(code) == challenge.CodeHash
}

func hashEmailOTPCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
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
