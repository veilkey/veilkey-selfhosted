package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

const (
	adminSessionCookieName         = "veilkey_admin_session"
	adminSessionTTLDefault         = 2 * time.Hour
	adminSessionIdleTimeoutDefault = 30 * time.Minute
	adminRevealWindow              = 5 * time.Minute
	adminTOTPPeriodSeconds         = 30
	adminTOTPDigits                = 6
)

func adminSessionTTL() time.Duration {
	return envDuration("VEILKEY_ADMIN_SESSION_TTL", adminSessionTTLDefault)
}

func adminSessionIdleTimeout() time.Duration {
	return envDuration("VEILKEY_ADMIN_SESSION_IDLE_TIMEOUT", adminSessionIdleTimeoutDefault)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("invalid duration %s=%q, using default %s", key, v, fallback)
		return fallback
	}
	return d
}

type adminAuthSettingsResponse struct {
	TOTPEnabled         bool  `json:"totp_enabled"`
	TOTPEnrolled        bool  `json:"totp_enrolled"`
	PendingEnrollment   bool  `json:"pending_enrollment"`
	SessionTTLSeconds   int64 `json:"session_ttl_seconds"`
	IdleTTLSeconds      int64 `json:"idle_timeout_seconds"`
	RevealWindowSeconds int64 `json:"reveal_window_seconds"`
}

func (h *Handler) handleAdminCreateSecureInputChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
		InputLabel  string `json:"input_label"`
		SubmitLabel string `json:"submit_label"`
		TargetName  string `json:"target_name"`
		BaseURL     string `json:"base_url"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	prompt := strings.TrimSpace(req.Prompt)
	if title == "" || prompt == "" {
		respondError(w, http.StatusBadRequest, "title and prompt are required")
		return
	}
	inputLabel := strings.TrimSpace(req.InputLabel)
	if inputLabel == "" {
		inputLabel = "Secure Input"
	}
	submitLabel := strings.TrimSpace(req.SubmitLabel)
	if submitLabel == "" {
		submitLabel = "Submit Secure Input"
	}
	token := crypto.GenerateUUID()
	challenge := &db.ApprovalTokenChallenge{
		Token:       token,
		Kind:        "secure_input",
		Title:       title,
		Prompt:      prompt,
		InputLabel:  inputLabel,
		SubmitLabel: submitLabel,
		TargetName:  strings.TrimSpace(req.TargetName),
		Status:      "pending",
	}
	if err := h.deps.DB().SaveApprovalTokenChallenge(challenge); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create approval challenge")
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = httputil.RequestBaseURL(r)
	}
	link := baseURL + "/approve/t/" + token
	h.deps.SaveAuditEvent("approval_token", token, "create", "admin", httputil.ActorIDForRequest(r), "secure_input", "admin_approval_token", nil, map[string]any{
		"kind":         challenge.Kind,
		"title":        challenge.Title,
		"target_name":  challenge.TargetName,
		"input_label":  challenge.InputLabel,
		"submit_label": challenge.SubmitLabel,
	})
	respondJSON(w, http.StatusCreated, map[string]any{
		"token":       token,
		"link":        link,
		"kind":        challenge.Kind,
		"status":      challenge.Status,
		"target_name": challenge.TargetName,
	})
}

func (h *Handler) handleAdminListApprovalChallenges(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	targetName := strings.TrimSpace(r.URL.Query().Get("target_name"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, total, err := h.deps.DB().ListApprovalTokenChallenges(targetName, kind, status, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load approval challenges")
		return
	}
	baseURL := httputil.RequestBaseURL(r)
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"token":       row.Token,
			"kind":        row.Kind,
			"title":       row.Title,
			"status":      row.Status,
			"target_name": row.TargetName,
			"created_at":  row.CreatedAt.UTC().Format(time.RFC3339),
			"used_at":     formatOptionalTime(row.UsedAt),
			"link":        strings.TrimRight(baseURL, "/") + "/approve/t/" + row.Token,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"challenges":  items,
		"count":       len(items),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) handleAdminAuthSettings(w http.ResponseWriter, r *http.Request) {
	cfg, _ := h.deps.DB().GetOrCreateAdminAuthConfig()
	respondJSON(w, http.StatusOK, adminAuthSettingsPayload(cfg))
}

func (h *Handler) handleAdminTOTPEnrollStart(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.deps.DB().GetOrCreateAdminAuthConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate totp secret")
		return
	}
	ciphertext, nonce, err := h.encryptAdminSecret(secret)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to protect totp secret")
		return
	}
	now := time.Now().UTC()
	cfg.PendingSecretCiphertext = ciphertext
	cfg.PendingSecretNonce = nonce
	cfg.PendingIssuedAt = &now
	if err := h.deps.DB().SaveAdminAuthConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save pending enrollment")
		return
	}

	otpauthURI := buildTOTPURI(secret)
	h.deps.SaveAuditEvent("admin_auth", "default", "totp_enroll_start", "api", httputil.ActorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"pending":      true,
		"issued_at":    now.Format(time.RFC3339),
		"totp_enabled": cfg.TOTPEnabled,
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_uri": otpauthURI,
		"settings":    adminAuthSettingsPayload(cfg),
	})
}

func (h *Handler) handleAdminTOTPEnrollVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	cfg, err := h.deps.DB().GetOrCreateAdminAuthConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	secret, err := h.decryptAdminSecret(cfg.PendingSecretCiphertext, cfg.PendingSecretNonce)
	if err != nil || secret == "" {
		respondError(w, http.StatusBadRequest, "pending totp enrollment not found")
		return
	}
	if !verifyTOTP(secret, req.Code, time.Now().UTC()) {
		h.deps.SaveAuditEvent("admin_auth", "default", "totp_enroll_verify_failed", "api", httputil.ActorIDForRequest(r), "invalid_code", "admin_auth", nil, nil)
		respondError(w, http.StatusUnauthorized, "invalid totp code")
		return
	}
	now := time.Now().UTC()
	cfg.TOTPEnabled = true
	cfg.EnrolledAt = &now
	cfg.TOTPSecretCiphertext = cfg.PendingSecretCiphertext
	cfg.TOTPSecretNonce = cfg.PendingSecretNonce
	cfg.PendingSecretCiphertext = nil
	cfg.PendingSecretNonce = nil
	cfg.PendingIssuedAt = nil
	if err := h.deps.DB().SaveAdminAuthConfig(cfg); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to finalize totp enrollment")
		return
	}
	h.deps.SaveAuditEvent("admin_auth", "default", "totp_enroll_verify", "api", httputil.ActorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"totp_enabled": true,
		"enrolled_at":  now.Format(time.RFC3339),
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"status":   "enrolled",
		"settings": adminAuthSettingsPayload(cfg),
	})
}

func (h *Handler) handleAdminSessionLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	cfg, err := h.deps.DB().GetOrCreateAdminAuthConfig()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	if !cfg.TOTPEnabled || len(cfg.TOTPSecretCiphertext) == 0 {
		respondError(w, http.StatusBadRequest, "admin totp is not configured")
		return
	}
	secret, err := h.decryptAdminSecret(cfg.TOTPSecretCiphertext, cfg.TOTPSecretNonce)
	if err != nil || secret == "" {
		respondError(w, http.StatusInternalServerError, "failed to load totp secret")
		return
	}
	if !verifyTOTP(secret, req.Code, time.Now().UTC()) {
		h.deps.SaveAuditEvent("admin_auth", "default", "session_login_failed", "api", httputil.ActorIDForRequest(r), "invalid_code", "admin_auth", nil, nil)
		respondError(w, http.StatusUnauthorized, "invalid totp code")
		return
	}

	token, tokenHash := generateAdminSessionToken()
	now := time.Now().UTC()
	session := &db.AdminSession{
		SessionID:     crypto.GenerateUUID(),
		TokenHash:     tokenHash,
		AuthMethod:    "totp",
		RemoteAddr:    httputil.ActorIDForRequest(r),
		ExpiresAt:     now.Add(adminSessionTTL()),
		IdleExpiresAt: now.Add(adminSessionIdleTimeout()),
		LastSeenAt:    now,
		CreatedAt:     now,
	}
	if err := h.deps.DB().SaveAdminSession(session); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create admin session")
		return
	}
	setAdminSessionCookie(w, token, session.ExpiresAt)
	h.deps.SaveAuditEvent("admin_session", session.SessionID, "session_login", "api", httputil.ActorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"auth_method":     session.AuthMethod,
		"expires_at":      session.ExpiresAt.Format(time.RFC3339),
		"idle_expires_at": session.IdleExpiresAt.Format(time.RFC3339),
	})
	respondJSON(w, http.StatusOK, sessionPayload(session))
}

func (h *Handler) handleAdminSessionGet(w http.ResponseWriter, r *http.Request) {
	session, err := h.currentAdminSession(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "admin session required")
		return
	}
	respondJSON(w, http.StatusOK, sessionPayload(session))
}

func (h *Handler) handleAdminSessionDelete(w http.ResponseWriter, r *http.Request) {
	session, err := h.currentAdminSession(r)
	if err == nil {
		_ = h.deps.DB().RevokeAdminSession(session.SessionID, time.Now().UTC())
		h.deps.SaveAuditEvent("admin_session", session.SessionID, "session_logout", "api", httputil.ActorIDForRequest(r), "", "admin_auth", nil, nil)
	}
	clearAdminSessionCookie(w)
	respondJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (h *Handler) handleAdminRecentAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListRecentAdminAuditEvents(limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load recent admin audit")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"events":      rows,
		"count":       len(rows),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) handleAdminRevealAuthorize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref    string `json:"ref"`
		Reason string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Ref) == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	session, _ := h.currentAdminSession(r)
	entry, err := h.deps.DB().GetSecretCatalogByRef(req.Ref)
	if err != nil {
		respondError(w, http.StatusNotFound, "secret catalog entry not found")
		return
	}
	now := time.Now().UTC()
	revealUntil := now.Add(adminRevealWindow)
	if err := h.deps.DB().UpdateAdminSessionRevealUntil(session.SessionID, &revealUntil); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to record reveal authorization")
		return
	}
	_ = h.deps.DB().MarkSecretCatalogRevealed(req.Ref, now)
	h.deps.SaveAuditEvent("secret", entry.RefCanonical, "admin_reveal_authorize", "admin_session", session.SessionID, strings.TrimSpace(req.Reason), "admin_auth", nil, map[string]any{
		"ref_canonical":  entry.RefCanonical,
		"vault_hash":     entry.VaultHash,
		"reveal_until":   revealUntil.Format(time.RFC3339),
		"authorized_at":  now.Format(time.RFC3339),
		"reason_present": strings.TrimSpace(req.Reason) != "",
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"authorized":            true,
		"ref":                   entry.RefCanonical,
		"reveal_window_seconds": int(adminRevealWindow.Seconds()),
		"reveal_until":          revealUntil.Format(time.RFC3339),
		"policy":                "authorization recorded; plaintext reveal remains explicit and separately auditable",
	})
}

func (h *Handler) handleAdminReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref string `json:"ref"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Ref) == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	session, _ := h.currentAdminSession(r)
	now := time.Now().UTC()
	if session.RevealUntil == nil || now.After(session.RevealUntil.UTC()) {
		respondError(w, http.StatusForbidden, "reveal authorization required")
		return
	}
	payload, err := h.resolveAdminReveal(req.Ref)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = h.deps.DB().MarkSecretCatalogRevealed(payload["ref"].(string), now)
	h.deps.SaveAuditEvent("secret", payload["ref"].(string), "admin_reveal", "admin_session", session.SessionID, "", "admin_auth", nil, map[string]any{
		"ref":           payload["ref"],
		"name":          payload["name"],
		"vault":         payload["vault"],
		"revealed_at":   now.Format(time.RFC3339),
		"reveal_until":  session.RevealUntil.UTC().Format(time.RFC3339),
		"value_present": true,
	})
	payload["reveal_until"] = session.RevealUntil.UTC().Format(time.RFC3339)
	payload["revealed_at"] = now.Format(time.RFC3339)
	respondJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleAdminRebindApprovalsList(w http.ResponseWriter, r *http.Request) {
	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	items := make([]map[string]any, 0)
	for _, agent := range agents {
		if !agent.RebindRequired && agent.BlockedAt == nil {
			continue
		}
		item := rebindApprovalPayload(&agent)
		latestEvidence, _ := h.deps.DB().GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "")
		if latestEvidence != nil {
			item["latest_secure_input"] = map[string]any{
				"token":      latestEvidence.Token,
				"status":     latestEvidence.Status,
				"created_at": latestEvidence.CreatedAt.UTC().Format(time.RFC3339),
				"used_at":    formatOptionalTime(latestEvidence.UsedAt),
			}
		}
		item["evidence_ready"] = latestEvidence != nil && latestEvidence.Status == "submitted" && latestEvidence.UsedAt != nil
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"approvals": items,
		"count":     len(items),
	})
}

func (h *Handler) handleAdminRebindPlan(w http.ResponseWriter, r *http.Request) {
	agent, err := h.deps.FindAgentRecord(r.PathValue("agent"))
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}
	latestEvidence, _ := h.deps.DB().GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "submitted")
	payload := map[string]any{
		"status":                "plan",
		"vault_runtime_hash":    agent.AgentHash,
		"vault_node_uuid":       agent.NodeID,
		"vault_id":              httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
		"current_key_version":   agent.KeyVersion,
		"next_key_version":      agent.KeyVersion + 1,
		"managed_paths":         db.DecodeManagedPaths(agent.ManagedPaths),
		"rebind_required":       agent.RebindRequired,
		"blocked":               agent.BlockedAt != nil,
		"rebind_reason":         agent.RebindReason,
		"block_reason":          agent.BlockReason,
		"secure_input_required": true,
	}
	if latestEvidence != nil {
		payload["latest_secure_input"] = map[string]any{
			"token":      latestEvidence.Token,
			"status":     latestEvidence.Status,
			"created_at": latestEvidence.CreatedAt.UTC().Format(time.RFC3339),
			"used_at":    formatOptionalTime(latestEvidence.UsedAt),
		}
	}
	respondJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleAdminApproveRebind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "approval payload is required")
		return
	}
	session, _ := h.currentAdminSession(r)
	agent, err := h.deps.FindAgentRecord(r.PathValue("agent"))
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	requiredConfirm := "APPROVE " + strings.ToUpper(strings.TrimSpace(agent.AgentHash))
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != requiredConfirm {
		respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	approvalReason := strings.TrimSpace(req.Reason)
	if approvalReason == "" {
		respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}
	evidence, err := h.deps.DB().GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "submitted")
	if err != nil || evidence == nil {
		respondError(w, http.StatusConflict, "submitted secure input evidence required")
		return
	}
	updated, err := h.deps.DB().ApproveAgentRebind(agent.NodeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to approve agent rebind: "+err.Error())
		return
	}
	h.deps.SaveAuditEvent(
		"vault",
		updated.NodeID,
		"admin_approve_rebind",
		"admin_session",
		session.SessionID,
		approvalReason,
		"admin_rebind_approval",
		map[string]any{
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rebind_required":    agent.RebindRequired,
			"blocked":            agent.BlockedAt != nil,
			"reason":             approvalReason,
			"evidence_token":     evidence.Token,
		},
		map[string]any{
			"vault_runtime_hash": updated.AgentHash,
			"key_version":        updated.KeyVersion,
			"rebind_required":    updated.RebindRequired,
			"blocked":            updated.BlockedAt != nil,
			"reason":             approvalReason,
			"evidence_token":     evidence.Token,
		},
	)
	respondJSON(w, http.StatusOK, map[string]any{
		"status":             "approved",
		"vault_runtime_hash": updated.AgentHash,
		"vault_node_uuid":    updated.NodeID,
		"vault_id":           httputil.FormatVaultID(updated.VaultName, updated.VaultHash),
		"managed_paths":      db.DecodeManagedPaths(updated.ManagedPaths),
		"key_version":        updated.KeyVersion,
		"reason":             approvalReason,
		"evidence_token":     evidence.Token,
	})
}

func (h *Handler) handleAdminScheduleAllRotations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "rotation payload is required")
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != "ROTATE ALL" {
		respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	session, _ := h.currentAdminSession(r)
	_, err := h.deps.DB().AdvancePendingRotations(time.Now().UTC())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to advance pending rotations: "+err.Error())
		return
	}
	agents, err := h.deps.DB().ScheduleAllAgentRotations(reason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to schedule agent rotation: "+err.Error())
		return
	}
	results := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		h.deps.SaveAuditEvent("vault", agent.NodeID, "admin_schedule_rotation", "admin_session", session.SessionID, reason, "admin_rotate_all", nil, map[string]any{
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rotation_required":  agent.RotationRequired,
			"reason":             reason,
		})
		results = append(results, map[string]any{
			"node_id":            agent.NodeID,
			"vault_node_uuid":    agent.NodeID,
			"label":              agent.Label,
			"vault_id":           httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rotation_required":  agent.RotationRequired,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "scheduled",
		"count":  len(results),
		"reason": reason,
		"agents": results,
	})
}

func (h *Handler) handleAdminScheduleRotation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "rotation payload is required")
		return
	}
	session, _ := h.currentAdminSession(r)
	agent, err := h.deps.FindAgentRecord(r.PathValue("agent"))
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	requiredConfirm := "ROTATE " + strings.ToUpper(strings.TrimSpace(agent.AgentHash))
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != requiredConfirm {
		respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if agent.RotationRequired {
		respondError(w, http.StatusBadRequest, "rotation already scheduled")
		return
	}
	updated, err := h.deps.DB().ScheduleAgentRotation(agent.NodeID, reason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to schedule agent rotation: "+err.Error())
		return
	}
	h.deps.SaveAuditEvent("vault", updated.NodeID, "admin_schedule_rotation_single", "admin_session", session.SessionID, reason, "admin_rotate_single", map[string]any{
		"vault_runtime_hash": agent.AgentHash,
		"key_version":        agent.KeyVersion,
		"rotation_required":  agent.RotationRequired,
	}, map[string]any{
		"vault_runtime_hash": updated.AgentHash,
		"key_version":        updated.KeyVersion,
		"rotation_required":  updated.RotationRequired,
		"reason":             reason,
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"status":             "scheduled",
		"vault_runtime_hash": updated.AgentHash,
		"vault_node_uuid":    updated.NodeID,
		"vault_id":           httputil.FormatVaultID(updated.VaultName, updated.VaultHash),
		"key_version":        updated.KeyVersion,
		"rotation_required":  updated.RotationRequired,
		"reason":             reason,
	})
}

func (h *Handler) requireAdminSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := h.currentAdminSession(r); err != nil {
			respondError(w, http.StatusUnauthorized, "admin session required")
			return
		}
		next(w, r)
	}
}

func (h *Handler) currentAdminSession(r *http.Request) (*db.AdminSession, error) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, fmt.Errorf("admin session cookie not found")
	}
	session, err := h.deps.DB().GetAdminSessionByTokenHash(hashAdminSessionToken(cookie.Value))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if session.RevokedAt != nil || now.After(session.ExpiresAt) || now.After(session.IdleExpiresAt) {
		if session.RevokedAt == nil {
			_ = h.deps.DB().RevokeAdminSession(session.SessionID, now)
		}
		return nil, fmt.Errorf("admin session expired")
	}
	session.LastSeenAt = now
	session.IdleExpiresAt = now.Add(adminSessionIdleTimeout())
	_ = h.deps.DB().TouchAdminSession(session.SessionID, session.LastSeenAt, session.IdleExpiresAt)
	return session, nil
}

func adminAuthSettingsPayload(cfg *db.AdminAuthConfig) adminAuthSettingsResponse {
	if cfg == nil {
		cfg = &db.AdminAuthConfig{}
	}
	return adminAuthSettingsResponse{
		TOTPEnabled:         cfg.TOTPEnabled,
		TOTPEnrolled:        cfg.EnrolledAt != nil && len(cfg.TOTPSecretCiphertext) > 0,
		PendingEnrollment:   cfg.PendingIssuedAt != nil && len(cfg.PendingSecretCiphertext) > 0,
		SessionTTLSeconds:   int64(adminSessionTTL().Seconds()),
		IdleTTLSeconds:      int64(adminSessionIdleTimeout().Seconds()),
		RevealWindowSeconds: int64(adminRevealWindow.Seconds()),
	}
}

func sessionPayload(session *db.AdminSession) map[string]any {
	return map[string]any{
		"session_id":           session.SessionID,
		"auth_method":          session.AuthMethod,
		"remote_addr":          session.RemoteAddr,
		"expires_at":           session.ExpiresAt.Format(time.RFC3339),
		"idle_expires_at":      session.IdleExpiresAt.Format(time.RFC3339),
		"last_seen_at":         session.LastSeenAt.Format(time.RFC3339),
		"reveal_until":         formatOptionalTime(session.RevealUntil),
		"session_ttl_seconds":  int64(adminSessionTTL().Seconds()),
		"idle_timeout_seconds": int64(adminSessionIdleTimeout().Seconds()),
	}
}

func formatOptionalTime(ts *time.Time) any {
	if ts == nil {
		return nil
	}
	return ts.UTC().Format(time.RFC3339)
}

func setAdminSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt.UTC(),
	})
}

func clearAdminSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
	})
}

func generateAdminSessionToken() (string, string) {
	key, _ := crypto.GenerateKey()
	token := base64.RawURLEncoding.EncodeToString(key)
	return token, hashAdminSessionToken(token)
}

func hashAdminSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (h *Handler) encryptAdminSecret(secret string) ([]byte, []byte, error) {
	dek, err := h.deps.GetLocalDEK()
	if err != nil {
		return nil, nil, err
	}
	return crypto.Encrypt(dek, []byte(secret))
}

func (h *Handler) decryptAdminSecret(ciphertext, nonce []byte) (string, error) {
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return "", nil
	}
	dek, err := h.deps.GetLocalDEK()
	if err != nil {
		return "", err
	}
	plaintext, err := crypto.Decrypt(dek, ciphertext, nonce)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func generateTOTPSecret() (string, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key[:20])
	return secret, nil
}

func buildTOTPURI(secret string) string {
	issuer := "VeilKey VaultCenter"
	account := "admin"
	label := url.PathEscape(issuer + ":" + account)
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("period", strconv.Itoa(adminTOTPPeriodSeconds))
	v.Set("digits", strconv.Itoa(adminTOTPDigits))
	v.Set("algorithm", "SHA256")
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func verifyTOTP(secret, code string, now time.Time) bool {
	normalized := strings.TrimSpace(code)
	if len(normalized) != adminTOTPDigits {
		return false
	}
	match := false
	for offset := -1; offset <= 1; offset++ {
		expected := totpCode(secret, now.Add(time.Duration(offset*adminTOTPPeriodSeconds)*time.Second))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(normalized)) == 1 {
			match = true
		}
	}
	return match
}

func totpCode(secret string, now time.Time) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := enc.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	counter := uint64(now.UTC().Unix() / adminTOTPPeriodSeconds)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < adminTOTPDigits; i++ {
		mod *= 10
	}
	value := truncated % mod
	return fmt.Sprintf("%0*d", adminTOTPDigits, value)
}

func decodeRequestJSON(r *http.Request, dst any) error {
	return httputil.DecodeJSON(r, dst)
}

func (h *Handler) resolveAdminReveal(ref string) (map[string]any, error) {
	tracked, err := h.deps.DB().GetRef(strings.TrimSpace(ref))
	if err != nil || tracked == nil {
		return nil, fmt.Errorf("tracked ref not found")
	}
	if tracked.AgentHash == "" {
		return nil, fmt.Errorf("tracked ref is not revealable")
	}
	agent, err := h.deps.DB().GetAgentByHash(tracked.AgentHash)
	if err != nil || len(agent.DEK) == 0 {
		return nil, fmt.Errorf("tracked ref vault is unavailable")
	}
	agentDEK, err := h.deps.DecryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to load vault key")
	}
	agentURL := h.deps.AgentURL(agent.IP, agent.Port)
	secretName, ciphertext, nonce, err := h.deps.FetchAgentCiphertext(agentURL, tracked.RefID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ciphertext")
	}
	plaintext, err := crypto.Decrypt(agentDEK, ciphertext, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret")
	}
	return map[string]any{
		"ref":                tracked.RefCanonical,
		"name":               secretName,
		"value":              string(plaintext),
		"vault":              agent.Label,
		"vault_runtime_hash": tracked.AgentHash,
	}, nil
}

func rebindApprovalPayload(agent *db.Agent) map[string]any {
	if agent == nil {
		return map[string]any{}
	}
	return map[string]any{
		"vault_runtime_hash":  agent.AgentHash,
		"vault_node_uuid":     agent.NodeID,
		"label":               agent.Label,
		"vault_hash":          agent.VaultHash,
		"vault_name":          agent.VaultName,
		"vault_id":            httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
		"current_key_version": agent.KeyVersion,
		"next_key_version":    agent.KeyVersion + 1,
		"rebind_required":     agent.RebindRequired,
		"rebind_reason":       agent.RebindReason,
		"blocked":             agent.BlockedAt != nil,
		"block_reason":        agent.BlockReason,
		"managed_paths":       db.DecodeManagedPaths(agent.ManagedPaths),
	}
}
