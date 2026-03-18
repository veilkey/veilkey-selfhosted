package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	vcrypto "veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

type secretInputRequest struct {
	Email      string `json:"email"`
	Endpoint   string `json:"endpoint"`
	Vault      string `json:"vault"`
	SecretName string `json:"secret_name"`
	Reason     string `json:"reason"`
	BaseURL    string `json:"base_url"`
}

type secretInputSubmitRequest struct {
	Token   string `json:"token"`
	Value   string `json:"value"`
	Confirm string `json:"confirm"`
}

func (s *Server) handleCreateSecretInputChallenge(w http.ResponseWriter, r *http.Request) {
	var req secretInputRequest
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.Vault = strings.TrimSpace(req.Vault)
	req.SecretName = strings.TrimSpace(req.SecretName)
	if req.Endpoint == "" || req.Vault == "" || req.SecretName == "" {
		s.respondError(w, http.StatusBadRequest, "endpoint, vault and secret_name are required")
		return
	}
	token := vcrypto.GenerateUUID()
	challenge := &db.SecretInputChallenge{
		Token:      token,
		Email:      req.Email,
		Endpoint:   req.Endpoint,
		Vault:      req.Vault,
		SecretName: req.SecretName,
		Reason:     strings.TrimSpace(req.Reason),
		Status:     "pending",
	}
	if err := s.db.SaveSecretInputChallenge(challenge); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}
	s.respondJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"link":  baseURL + "/ui/approvals/secret-input?token=" + token,
	})
}

func (s *Server) handleSecretInputPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		s.respondError(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := s.db.GetSecretInputChallenge(token)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if challenge.Status == "submitted" {
		s.respondError(w, http.StatusGone, "challenge already used")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	emailLabel := challenge.Email
	if strings.TrimSpace(emailLabel) == "" {
		emailLabel = "-"
	}
	fmt.Fprintf(w, secretInputHTML, emailLabel, challenge.Vault, challenge.SecretName, defaultSecretInputReason(challenge.Reason), token)
}

func (s *Server) handleSubmitSecretInput(w http.ResponseWriter, r *http.Request) {
	var req secretInputSubmitRequest
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		if err := decodeJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid form body")
			return
		}
		req.Token = r.FormValue("token")
		req.Value = r.FormValue("value")
		req.Confirm = r.FormValue("confirm")
	}
	if strings.TrimSpace(req.Token) == "" || req.Value == "" || req.Confirm == "" {
		s.respondError(w, http.StatusBadRequest, "token, value and confirm are required")
		return
	}
	if req.Value != req.Confirm {
		s.respondError(w, http.StatusBadRequest, "value and confirm must match")
		return
	}
	challenge, err := s.db.GetSecretInputChallenge(strings.TrimSpace(req.Token))
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if challenge.Status == "submitted" {
		s.respondError(w, http.StatusGone, "challenge already used")
		return
	}
	if err := storeSecretViaAgentEndpoint(challenge.Endpoint, challenge.SecretName, req.Value); err != nil {
		s.respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	if _, err := s.db.CompleteSecretInputChallenge(challenge.Token); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.db.SaveAuditEvent(&db.AuditEvent{
		EventID:    vcrypto.GenerateUUID(),
		EntityType: "secret_input",
		EntityID:   challenge.Token,
		Action:     "submit",
		ActorType:  "user",
		ActorID:    challenge.Email,
		Reason:     "secret_input_submitted",
		Source:     "vaultcenter_ui",
	})
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		s.respondJSON(w, http.StatusOK, map[string]any{
			"status":      "submitted",
			"secret_name": challenge.SecretName,
			"vault":       challenge.Vault,
		})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, secretInputSuccessHTML)
}

func storeSecretViaAgentEndpoint(endpoint, name, value string) error {
	target := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.Contains(target, "/api/agents/") {
		return fmt.Errorf("secret input requires a vaultcenter agent endpoint")
	}
	payload, err := json.Marshal(map[string]string{
		"name":  name,
		"value": value,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal secret payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, target+"/secrets", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := InitHTTPClientFromEnv().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("store secret failed: (unreadable body)")
		}
		return fmt.Errorf("store secret failed: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func defaultSecretInputReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "secret input"
	}
	return reason
}

const secretInputHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>VeilKey Secret Input</title>
  <style>
    body { margin: 0; font-family: sans-serif; background: #f6f4ee; color: #1b1d1b; }
    .wrap { max-width: 640px; margin: 8vh auto; padding: 24px; }
    .card { background: #fffdf8; border: 1px solid #ddd8ca; padding: 28px; }
    h1 { margin: 0 0 14px; font-size: 30px; }
    p { line-height: 1.55; }
    .meta { color: #5d665d; font-size: 14px; }
    input { width: 100%%; box-sizing: border-box; padding: 14px; font-size: 18px; border: 1px solid #d8d0be; margin: 10px 0 14px; }
    button { border: 0; background: #1b1d1b; color: white; padding: 12px 18px; font-size: 16px; cursor: pointer; }
  </style>
</head>
<body><div class="wrap"><div class="card">
<h1>Secure Secret Input</h1>
<p class="meta">Target email: <strong>%s</strong></p>
<p class="meta">Target vault: <strong>%s</strong></p>
<p class="meta">Target name: <strong>%s</strong></p>
<p class="meta">Purpose: %s</p>
<form method="post" action="/ui/approvals/secret-input">
  <input type="hidden" name="token" value="%s">
  <input type="password" name="value" placeholder="secret value" autofocus>
  <input type="password" name="confirm" placeholder="confirm value">
  <button type="submit">Store secret</button>
</form>
</div></div></body>
</html>`

const secretInputSuccessHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>VeilKey Secret Input</title></head>
<body><div style="max-width:640px;margin:8vh auto;padding:24px;font-family:sans-serif"><h1>Secret stored</h1><p>The secret was stored in VeilKey successfully. Terminal re-entry is not required.</p></div></body>
</html>`
