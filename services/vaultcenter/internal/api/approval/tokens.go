package approval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	vcrypto "veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

type approvalTokenSubmitRequest struct {
	Token string `json:"token"`
	Value string `json:"value"`
}

func (h *Handler) handleApprovalTokenPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	if challenge, err := h.db.GetInstallCustodyChallenge(token); err == nil && challenge != nil {
		q := r.URL.Query()
		q.Set("token", token)
		r.URL.RawQuery = q.Encode()
		h.custody.HandleInstallCustodyPage(w, r)
		return
	}
	if challenge, err := h.db.GetApprovalTokenChallenge(token); err == nil && challenge != nil {
		if challenge.Status == "submitted" {
			respondErr(w, http.StatusGone, "challenge already used")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, secureInputApprovalHTML,
			escapeApprovalHTML(challenge.Title),
			escapeApprovalHTML(challenge.Title),
			escapeApprovalHTML(challenge.Prompt),
			escapeApprovalHTML(token),
			escapeApprovalHTML(challenge.InputLabel),
			escapeApprovalHTML(challenge.SubmitLabel),
		)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, approvalTokenPlaceholderHTML, escapeApprovalHTML(token))
}

func (h *Handler) handleApprovalTokenSubmit(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		respondErr(w, http.StatusBadRequest, "token is required")
		return
	}
	if challenge, err := h.db.GetInstallCustodyChallenge(token); err == nil && challenge != nil {
		if err := r.ParseForm(); err == nil {
			if strings.TrimSpace(r.FormValue("token")) == "" {
				r.Form.Set("token", token)
			}
		}
		h.custody.HandleSubmitInstallCustody(w, r)
		_ = challenge
		return
	}
	if challenge, err := h.db.GetApprovalTokenChallenge(token); err == nil && challenge != nil {
		h.handleApprovalTokenChallengeSubmit(w, r, challenge)
		return
	}
	respondErr(w, http.StatusNotFound, "approval token not found")
}

func (h *Handler) handleApprovalTokenChallengeSubmit(w http.ResponseWriter, r *http.Request, challenge *db.ApprovalTokenChallenge) {
	if challenge.Status == "submitted" {
		respondErr(w, http.StatusGone, "challenge already used")
		return
	}

	var req approvalTokenSubmitRequest
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
		req.Token = strings.TrimSpace(r.FormValue("token"))
		req.Value = strings.TrimSpace(r.FormValue("value"))
	}
	if req.Token == "" {
		req.Token = challenge.Token
	}
	if req.Token != challenge.Token || strings.TrimSpace(req.Value) == "" {
		respondErr(w, http.StatusBadRequest, "token and value are required")
		return
	}
	key := deriveApprovalTokenKey(h.salt, challenge.Token, challenge.Kind)
	ciphertext, nonce, err := vcrypto.Encrypt(key, []byte(req.Value))
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to protect submitted value")
		return
	}
	if _, err := h.db.CompleteApprovalTokenChallenge(challenge.Token, ciphertext, nonce); err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if challenge.Kind == "install_bootstrap" && challenge.TargetName != "" {
		if session, err := h.db.GetInstallSession(challenge.TargetName); err == nil {
			completed := appendUniqueStage(httputil.DecodeStringList(session.CompletedStagesJSON), "bootstrap")
			session.CompletedStagesJSON = httputil.EncodeStringList(completed)
			session.LastStage = "bootstrap"
			_ = h.db.SaveInstallSession(session)
		}
	}
	after := map[string]any{
		"kind":        challenge.Kind,
		"target_name": challenge.TargetName,
		"status":      "submitted",
		"used_at":     time.Now().UTC().Format(time.RFC3339),
	}
	_ = h.db.SaveAuditEvent(&db.AuditEvent{
		EventID:             vcrypto.GenerateUUID(),
		EntityType:          "approval_token",
		EntityID:            challenge.Token,
		Action:              "submit",
		ActorType:           "user",
		ActorID:             httputil.ActorIDForRequest(r),
		Reason:              challenge.Kind,
		Source:              "approval_token",
		ApprovalChallengeID: challenge.Token,
		BeforeJSON:          "{}",
		AfterJSON:           mustMarshalJSON(after),
		CreatedAt:           time.Now().UTC(),
	})
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		respond(w, http.StatusOK, map[string]any{
			"status":      "submitted",
			"token":       challenge.Token,
			"kind":        challenge.Kind,
			"target_name": challenge.TargetName,
		})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, secureInputApprovalSuccessHTML, escapeApprovalHTML(challenge.Title), escapeApprovalHTML(challenge.Title))
}

func appendUniqueStage(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func deriveApprovalTokenKey(salt []byte, token, kind string) []byte {
	sum := sha256.Sum256(append(append(append([]byte{}, salt...), []byte(kind)...), []byte(token)...))
	return sum[:]
}

func mustMarshalJSON(value map[string]any) string {
	if len(value) == 0 {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func escapeApprovalHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

const secureInputApprovalHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 16px;color:#111827}
.card{border:1px solid #d1d5db;border-radius:12px;padding:24px}
label{display:block;font-weight:600;margin:16px 0 8px}
input,button{font:inherit}
input[type=text],input[type=password]{width:100%%;padding:12px 14px;border:1px solid #d1d5db;border-radius:10px;box-sizing:border-box}
button{margin-top:16px;background:#111827;color:#fff;border:none;border-radius:10px;padding:12px 16px;cursor:pointer}
</style></head><body>
<div class="card">
<h1>%s</h1>
<p>%s</p>
<form method="post">
<input type="hidden" name="token" value="%s">
<label for="value">%s</label>
<input id="value" type="password" name="value" autocomplete="off" required>
<button type="submit">%s</button>
</form>
</div></body></html>
`

const secureInputApprovalSuccessHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s Complete</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 16px;color:#111827}
.card{border:1px solid #d1d5db;border-radius:12px;padding:24px}
</style></head><body>
<div class="card">
<h1>%s</h1>
<p>The secure input was stored successfully. You can return to the approval flow.</p>
</div></body></html>
`

const approvalTokenPlaceholderHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VeilKey Approval</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 16px;color:#111827}
.card{border:1px solid #d1d5db;border-radius:12px;padding:24px}
code{background:#f3f4f6;padding:2px 6px;border-radius:6px}
</style></head><body>
<div class="card">
<h1>VeilKey Approval</h1>
<p>This token route is now owned by VaultCenter.</p>
<p>Token: <code>%s</code></p>
<p>The token did not match a migrated approval challenge yet.</p>
<p>Current canonical routes already moved:</p>
<ul>
<li><code>/approve/install/bootstrap</code> for install/bootstrap confirmation input</li>
<li><code>/approve/install/custody</code> for install/bootstrap custody input</li>
<li><code>/approve/t/{token}</code> for generic secure input challenges</li>
<li>root VaultCenter console for rebind approvals and admin actions</li>
</ul>
</div></body></html>
`
