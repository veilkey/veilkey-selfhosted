package install

import (
	"fmt"
	"net/http"
	"strings"

	vcrypto "github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
	"veilkey-vaultcenter/internal/mailer"
)

type installBootstrapRequest struct {
	SessionID string `json:"session_id"`
	Email     string `json:"email"`
	BaseURL   string `json:"base_url"`
}

func (h *Handler) handleCreateInstallBootstrapChallenge(w http.ResponseWriter, r *http.Request) {
	var req installBootstrapRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" || req.Email == "" {
		respondErr(w, http.StatusBadRequest, "session_id and email are required")
		return
	}
	session, err := h.db.GetInstallSession(req.SessionID)
	if err != nil {
		respondErr(w, http.StatusNotFound, err.Error())
		return
	}
	token := vcrypto.GenerateUUID()
	challenge := &db.ApprovalTokenChallenge{
		Token:       token,
		Kind:        "install_bootstrap",
		Title:       "Install Bootstrap Confirmation",
		Prompt:      "Provide the bootstrap confirmation or OTP that authorizes this install session to proceed.",
		InputLabel:  "Bootstrap Confirmation",
		SubmitLabel: "Store Bootstrap Confirmation",
		TargetName:  req.SessionID,
		Status:      "pending",
	}
	if err := h.db.SaveApprovalTokenChallenge(challenge); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to create install bootstrap challenge")
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = httputil.RequestBaseURL(r)
	}
	link := baseURL + "/approve/t/" + token
	subject := "VeilKey install bootstrap input"
	body := fmt.Sprintf(
		"Open the link below to provide bootstrap confirmation for the install session.\n\nSession: %s\nFlow: %s\nLink: %s\n",
		session.SessionID,
		session.Flow,
		link,
	)
	if err := mailer.Send(req.Email, subject, body); err != nil {
		respondErr(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.db.SaveAuditEvent(&db.AuditEvent{
		EventID:             vcrypto.GenerateUUID(),
		EntityType:          "install_bootstrap",
		EntityID:            token,
		Action:              "request",
		ActorType:           "api",
		ActorID:             httputil.ActorIDForRequest(r),
		Reason:              "install_bootstrap_request",
		Source:              "install_bootstrap",
		ApprovalChallengeID: token,
	})
	respond(w, http.StatusCreated, map[string]any{
		"token":      token,
		"link":       link,
		"session_id": session.SessionID,
		"kind":       challenge.Kind,
	})
}

func (h *Handler) handleInstallBootstrapPage(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	var session *db.InstallSession
	var err error
	if sessionID != "" {
		session, err = h.db.GetInstallSession(sessionID)
	} else {
		session, err = h.db.GetLatestInstallSession()
	}
	if err != nil {
		respondErr(w, http.StatusNotFound, "install session not found")
		return
	}
	challenge, err := h.db.GetLatestApprovalTokenChallenge(session.SessionID, "install_bootstrap", "pending")
	if err != nil || challenge == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, installBootstrapPendingHTML)
		return
	}
	http.Redirect(w, r, "/approve/t/"+challenge.Token, http.StatusSeeOther)
}

const installBootstrapPendingHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>VeilKey Install Bootstrap</title>
<style>
body{font-family:-apple-system,system-ui,sans-serif;max-width:760px;margin:40px auto;padding:0 16px;color:#111827}
.card{border:1px solid #d1d5db;border-radius:12px;padding:24px}
</style></head><body>
<div class="card">
<h1>Install Bootstrap</h1>
<p>No pending bootstrap approval link was found for the current install session.</p>
</div></body></html>
`
