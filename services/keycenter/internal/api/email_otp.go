package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	vcrypto "veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

type emailOTPRequest struct {
	Email   string `json:"email"`
	Reason  string `json:"reason"`
	BaseURL string `json:"base_url"`
}

func (s *Server) handleCreateEmailOTPChallenge(w http.ResponseWriter, r *http.Request) {
	var req emailOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		s.respondError(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.TrimSpace(req.Email)
	if !strings.Contains(email, "@") || !strings.Contains(email[strings.Index(email, "@"):], ".") {
		s.respondError(w, http.StatusBadRequest, "invalid email format")
		return
	}
	token := vcrypto.GenerateUUID()
	challenge := &db.EmailOTPChallenge{
		Token:  token,
		Email:  strings.TrimSpace(req.Email),
		Reason: strings.TrimSpace(req.Reason),
		Status: "pending",
	}
	if err := s.db.SaveEmailOTPChallenge(challenge); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}
	link := baseURL + "/ui/approvals/email-otp?token=" + token
	body := strings.Join([]string{
		fmt.Sprintf("VeilKey verification link for %s", requestBaseURL(r)),
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
	if err := sendInstallMail(challenge.Email, "VeilKey verification code", body); err != nil {
		s.respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.respondJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"link":  link,
	})
}

func (s *Server) handleEmailOTPState(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := s.db.GetEmailOTPChallenge(token)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"token":  challenge.Token,
		"email":  challenge.Email,
		"reason": challenge.Reason,
		"status": challenge.Status,
	})
}

func (s *Server) handleEmailOTPPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := s.db.GetEmailOTPChallenge(token)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
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

func (s *Server) handleSubmitEmailOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid form body")
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	action := strings.TrimSpace(r.FormValue("action"))
	if token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := s.db.GetEmailOTPChallenge(token)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	switch action {
	case "send-code":
		code := fmt.Sprintf("%06d", rand.IntN(1000000))
		expiresAt := time.Now().UTC().Add(5 * time.Minute)
		if _, err := s.db.UpdateEmailOTPCode(token, hashEmailOTPCode(code), expiresAt); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		body := fmt.Sprintf("VeilKey one-time code\n\nCode: %s\nExpires in: 300 seconds\n", code)
		if err := sendInstallMail(challenge.Email, "VeilKey one-time code", body); err != nil {
			s.respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, emailOTPCodeHTML, challenge.Email, defaultEmailOTPReason(challenge.Reason), token)
	case "verify":
		code := strings.TrimSpace(r.FormValue("code"))
		if !validateEmailOTPChallenge(challenge, code) {
			s.respondError(w, http.StatusForbidden, "code is invalid or expired")
			return
		}
		if _, err := s.db.MarkEmailOTPVerified(token); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, emailOTPSuccessHTML)
	default:
		s.respondError(w, http.StatusBadRequest, "unsupported action")
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
