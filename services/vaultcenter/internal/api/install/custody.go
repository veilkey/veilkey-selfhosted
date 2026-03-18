package install

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	vcrypto "veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
	"veilkey-vaultcenter/internal/mailer"
)

type installCustodyRequest struct {
	SessionID  string `json:"session_id"`
	Email      string `json:"email"`
	SecretName string `json:"secret_name"`
	BaseURL    string `json:"base_url"`
}

type installCustodySubmitRequest struct {
	Token string `json:"token"`
	Value string `json:"value"`
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func (h *Handler) handleCreateInstallCustodyChallenge(w http.ResponseWriter, r *http.Request) {
	var req installCustodyRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Email = strings.TrimSpace(req.Email)
	req.SecretName = strings.TrimSpace(req.SecretName)
	if req.SessionID == "" || req.SecretName == "" {
		respondErr(w, http.StatusBadRequest, "session_id and secret_name are required")
		return
	}
	if _, err := h.db.GetInstallSession(req.SessionID); err != nil {
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	token := vcrypto.GenerateUUID()
	challenge := &db.InstallCustodyChallenge{
		Token:      token,
		SessionID:  req.SessionID,
		Email:      req.Email,
		SecretName: req.SecretName,
		Status:     "pending",
	}
	if err := h.db.SaveInstallCustodyChallenge(challenge); err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = httputil.RequestBaseURL(r)
	}
	link := baseURL + "/approve/install/custody?token=" + token
	if req.Email != "" {
		subject := "VeilKey install custody input"
		body := fmt.Sprintf(
			"Open the link below to provide the first-install custody value.\n\nSession: %s\nTarget name: %s\nLink: %s\n",
			req.SessionID,
			req.SecretName,
			link,
		)
		if err := mailer.Send(req.Email, subject, body); err != nil {
			respondErr(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	respond(w, http.StatusCreated, map[string]any{
		"token": token,
		"link":  link,
	})
}

func (h *Handler) HandleInstallCustodyPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	challenge, err := h.db.GetInstallCustodyChallenge(token)
	if err != nil {
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	if challenge.Status == "submitted" {
		respondErr(w, http.StatusGone, "challenge already used")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	emailLabel := challenge.Email
	if strings.TrimSpace(emailLabel) == "" {
		emailLabel = "-"
	}
	fmt.Fprintf(w, installCustodyHTML, challenge.SecretName, emailLabel, token)
}

func (h *Handler) HandleSubmitInstallCustody(w http.ResponseWriter, r *http.Request) {
	var req installCustodySubmitRequest
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		if err := httputil.DecodeJSON(r, &req); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid form body")
			return
		}
		req.Token = r.FormValue("token")
		req.Value = r.FormValue("value")
	}
	if req.Token == "" || req.Value == "" {
		respondErr(w, http.StatusBadRequest, "token and value are required")
		return
	}
	challenge, err := h.db.GetInstallCustodyChallenge(req.Token)
	if err != nil {
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	if challenge.Status == "submitted" {
		respondErr(w, http.StatusGone, "challenge already used")
		return
	}
	key := deriveInstallCustodyKey(h.salt, req.Token)
	ciphertext, nonce, err := vcrypto.Encrypt(key, []byte(req.Value))
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to protect submitted value")
		return
	}
	if _, err := h.db.CompleteInstallCustodyChallenge(req.Token, ciphertext, nonce); err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	session, err := h.db.GetInstallSession(challenge.SessionID)
	if err == nil {
		completed := appendUnique(httputil.DecodeStringList(session.CompletedStagesJSON), "custody")
		session.CompletedStagesJSON = httputil.EncodeStringList(completed)
		session.LastStage = "custody"
		_ = h.db.SaveInstallSession(session)
	}
	_ = h.db.SaveAuditEvent(&db.AuditEvent{
		EventID:    vcrypto.GenerateUUID(),
		EntityType: "install_custody",
		EntityID:   req.Token,
		Action:     "submit",
		ActorType:  "user",
		ActorID:    challenge.Email,
		Reason:     "install_custody_submitted",
		Source:     "vaultcenter_ui",
	})
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		respond(w, http.StatusOK, map[string]any{
			"status":     "submitted",
			"session_id": challenge.SessionID,
		})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, installCustodySuccessHTML)
}

func deriveInstallCustodyKey(salt []byte, token string) []byte {
	sum := sha256.Sum256(append(append([]byte{}, salt...), []byte(token)...))
	return sum[:]
}

const installCustodyHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VeilKey Install Custody</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;max-width:720px;margin:40px auto;padding:0 16px;color:#111827}
.card{border:1px solid #d1d5db;border-radius:10px;padding:20px}
label{display:block;margin-top:12px;font-weight:600}
input,textarea,button{width:100%%;padding:10px;margin-top:6px}
button{cursor:pointer}
code{background:#f3f4f6;padding:2px 6px;border-radius:6px}
</style></head><body>
<div class="card">
<h1>VeilKey Install Custody</h1>
<p>Provide the initial custody value for <code>%s</code>.</p>
<p>Recipient: <code>%s</code></p>
<form method="post" action="/approve/install/custody">
<input type="hidden" name="token" value="%s">
<label for="value">Value</label>
<textarea id="value" name="value" rows="6" required></textarea>
<button type="submit">Store custody value</button>
</form>
</div></body></html>
`

const installCustodySuccessHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VeilKey Install Custody</title></head>
<body style="font-family:-apple-system,system-ui,sans-serif;max-width:680px;margin:40px auto;padding:0 16px">
<h1>Stored</h1>
<p>The install custody value was stored successfully. You can return to the install flow.</p>
</body></html>
`
