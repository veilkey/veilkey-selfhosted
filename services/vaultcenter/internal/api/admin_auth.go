package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

const (
	adminSessionCookieName  = "veilkey_admin_session"
	adminSessionTTL         = 2 * time.Hour
	adminSessionIdleTimeout = 30 * time.Minute
	adminRevealWindow       = 5 * time.Minute
	adminTOTPPeriodSeconds  = 30
	adminTOTPDigits         = 6
)

type adminAuthSettingsResponse struct {
	TOTPEnabled         bool  `json:"totp_enabled"`
	TOTPEnrolled        bool  `json:"totp_enrolled"`
	PendingEnrollment   bool  `json:"pending_enrollment"`
	SessionTTLSeconds   int64 `json:"session_ttl_seconds"`
	IdleTTLSeconds      int64 `json:"idle_timeout_seconds"`
	RevealWindowSeconds int64 `json:"reveal_window_seconds"`
}

func (s *Server) SetupAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/auth/settings", s.requireReadyForOps(s.handleAdminAuthSettings))
	mux.HandleFunc("POST /api/admin/auth/totp/enroll/start", s.requireTrustedIP(s.requireReadyForOps(s.handleAdminTOTPEnrollStart)))
	mux.HandleFunc("POST /api/admin/auth/totp/enroll/verify", s.requireTrustedIP(s.requireReadyForOps(s.handleAdminTOTPEnrollVerify)))
	mux.HandleFunc("POST /api/admin/session/login", s.requireReadyForOps(s.handleAdminSessionLogin))
	mux.HandleFunc("GET /api/admin/session", s.requireReadyForOps(s.handleAdminSessionGet))
	mux.HandleFunc("DELETE /api/admin/session", s.requireReadyForOps(s.handleAdminSessionDelete))
	mux.HandleFunc("GET /api/admin/approval-challenges", s.requireReadyForOps(s.requireAdminSession(s.handleAdminListApprovalChallenges)))
	mux.HandleFunc("POST /api/admin/approval-challenges/secure-input", s.requireTrustedIP(s.requireReadyForOps(s.requireAdminSession(s.handleAdminCreateSecureInputChallenge))))
	mux.HandleFunc("GET /api/admin/audit/recent", s.requireReadyForOps(s.requireAdminSession(s.handleAdminRecentAudit)))
	mux.HandleFunc("GET /api/admin/approvals/rebind", s.requireReadyForOps(s.requireAdminSession(s.handleAdminRebindApprovalsList)))
	mux.HandleFunc("GET /api/admin/approvals/rebind/{agent}", s.requireReadyForOps(s.requireAdminSession(s.handleAdminRebindPlan)))
	mux.HandleFunc("POST /api/admin/approvals/rebind/{agent}/approve", s.requireReadyForOps(s.requireAdminSession(s.handleAdminApproveRebind)))
	mux.HandleFunc("POST /api/admin/rotations/{agent}/schedule", s.requireReadyForOps(s.requireAdminSession(s.handleAdminScheduleRotation)))
	mux.HandleFunc("POST /api/admin/rotations/schedule-all", s.requireReadyForOps(s.requireAdminSession(s.handleAdminScheduleAllRotations)))
	mux.HandleFunc("POST /api/admin/tracked-refs/cleanup-preview", s.requireReadyForOps(s.requireAdminSession(s.handleAdminTrackedRefCleanupPreview)))
	mux.HandleFunc("POST /api/admin/tracked-refs/cleanup-apply", s.requireReadyForOps(s.requireAdminSession(s.handleAdminTrackedRefCleanupApply)))
	mux.HandleFunc("POST /api/admin/reveal-authorize", s.requireReadyForOps(s.requireAdminSession(s.handleAdminRevealAuthorize)))
	mux.HandleFunc("POST /api/admin/reveal", s.requireReadyForOps(s.requireAdminSession(s.handleAdminReveal)))
}

func (s *Server) handleAdminCreateSecureInputChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Prompt      string `json:"prompt"`
		InputLabel  string `json:"input_label"`
		SubmitLabel string `json:"submit_label"`
		TargetName  string `json:"target_name"`
		BaseURL     string `json:"base_url"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	prompt := strings.TrimSpace(req.Prompt)
	if title == "" || prompt == "" {
		s.respondError(w, http.StatusBadRequest, "title and prompt are required")
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
	if err := s.db.SaveApprovalTokenChallenge(challenge); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to create approval challenge")
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}
	link := baseURL + "/approve/t/" + token
	s.saveAuditEvent("approval_token", token, "create", "admin", actorIDForRequest(r), "secure_input", "admin_approval_token", nil, map[string]any{
		"kind":         challenge.Kind,
		"title":        challenge.Title,
		"target_name":  challenge.TargetName,
		"input_label":  challenge.InputLabel,
		"submit_label": challenge.SubmitLabel,
	})
	s.respondJSON(w, http.StatusCreated, map[string]any{
		"token":       token,
		"link":        link,
		"kind":        challenge.Kind,
		"status":      challenge.Status,
		"target_name": challenge.TargetName,
	})
}

func (s *Server) handleAdminListApprovalChallenges(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	targetName := strings.TrimSpace(r.URL.Query().Get("target_name"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, total, err := s.db.ListApprovalTokenChallenges(targetName, kind, status, limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load approval challenges")
		return
	}
	baseURL := requestBaseURL(r)
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
	s.respondJSON(w, http.StatusOK, map[string]any{
		"challenges":  items,
		"count":       len(items),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (s *Server) handleAdminAuthSettings(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.db.GetOrCreateAdminAuthConfig()
	s.respondJSON(w, http.StatusOK, adminAuthSettingsPayload(cfg))
}

func (s *Server) handleAdminTOTPEnrollStart(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetOrCreateAdminAuthConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate totp secret")
		return
	}
	ciphertext, nonce, err := s.encryptAdminSecret(secret)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to protect totp secret")
		return
	}
	now := time.Now().UTC()
	cfg.PendingSecretCiphertext = ciphertext
	cfg.PendingSecretNonce = nonce
	cfg.PendingIssuedAt = &now
	if err := s.db.SaveAdminAuthConfig(cfg); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save pending enrollment")
		return
	}

	otpauthURI := buildTOTPURI(secret)
	s.saveAuditEvent("admin_auth", "default", "totp_enroll_start", "api", actorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"pending":      true,
		"issued_at":    now.Format(time.RFC3339),
		"totp_enabled": cfg.TOTPEnabled,
	})
	s.respondJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_uri": otpauthURI,
		"settings":    adminAuthSettingsPayload(cfg),
	})
}

func (s *Server) handleAdminTOTPEnrollVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		s.respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	cfg, err := s.db.GetOrCreateAdminAuthConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	secret, err := s.decryptAdminSecret(cfg.PendingSecretCiphertext, cfg.PendingSecretNonce)
	if err != nil || secret == "" {
		s.respondError(w, http.StatusBadRequest, "pending totp enrollment not found")
		return
	}
	if !verifyTOTP(secret, req.Code, time.Now().UTC()) {
		s.saveAuditEvent("admin_auth", "default", "totp_enroll_verify_failed", "api", actorIDForRequest(r), "invalid_code", "admin_auth", nil, nil)
		s.respondError(w, http.StatusUnauthorized, "invalid totp code")
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
	if err := s.db.SaveAdminAuthConfig(cfg); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to finalize totp enrollment")
		return
	}
	s.saveAuditEvent("admin_auth", "default", "totp_enroll_verify", "api", actorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"totp_enabled": true,
		"enrolled_at":  now.Format(time.RFC3339),
	})
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":   "enrolled",
		"settings": adminAuthSettingsPayload(cfg),
	})
}

func (s *Server) handleAdminSessionLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		s.respondError(w, http.StatusBadRequest, "code is required")
		return
	}
	cfg, err := s.db.GetOrCreateAdminAuthConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load admin auth config")
		return
	}
	if !cfg.TOTPEnabled || len(cfg.TOTPSecretCiphertext) == 0 {
		s.respondError(w, http.StatusBadRequest, "admin totp is not configured")
		return
	}
	secret, err := s.decryptAdminSecret(cfg.TOTPSecretCiphertext, cfg.TOTPSecretNonce)
	if err != nil || secret == "" {
		s.respondError(w, http.StatusInternalServerError, "failed to load totp secret")
		return
	}
	if !verifyTOTP(secret, req.Code, time.Now().UTC()) {
		s.saveAuditEvent("admin_auth", "default", "session_login_failed", "api", actorIDForRequest(r), "invalid_code", "admin_auth", nil, nil)
		s.respondError(w, http.StatusUnauthorized, "invalid totp code")
		return
	}

	token, tokenHash := generateAdminSessionToken()
	now := time.Now().UTC()
	session := &db.AdminSession{
		SessionID:     crypto.GenerateUUID(),
		TokenHash:     tokenHash,
		AuthMethod:    "totp",
		RemoteAddr:    actorIDForRequest(r),
		ExpiresAt:     now.Add(adminSessionTTL),
		IdleExpiresAt: now.Add(adminSessionIdleTimeout),
		LastSeenAt:    now,
		CreatedAt:     now,
	}
	if err := s.db.SaveAdminSession(session); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to create admin session")
		return
	}
	setAdminSessionCookie(w, token, session.ExpiresAt)
	s.saveAuditEvent("admin_session", session.SessionID, "session_login", "api", actorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"auth_method":     session.AuthMethod,
		"expires_at":      session.ExpiresAt.Format(time.RFC3339),
		"idle_expires_at": session.IdleExpiresAt.Format(time.RFC3339),
	})
	s.respondJSON(w, http.StatusOK, sessionPayload(session))
}

func (s *Server) handleAdminSessionGet(w http.ResponseWriter, r *http.Request) {
	session, err := s.currentAdminSession(r)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, "admin session required")
		return
	}
	s.respondJSON(w, http.StatusOK, sessionPayload(session))
}

func (s *Server) handleAdminSessionDelete(w http.ResponseWriter, r *http.Request) {
	session, err := s.currentAdminSession(r)
	if err == nil {
		_ = s.db.RevokeAdminSession(session.SessionID, time.Now().UTC())
		s.saveAuditEvent("admin_session", session.SessionID, "session_logout", "api", actorIDForRequest(r), "", "admin_auth", nil, nil)
	}
	clearAdminSessionCookie(w)
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (s *Server) handleAdminRecentAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListRecentAdminAuditEvents(limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load recent admin audit")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"events":      rows,
		"count":       len(rows),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (s *Server) handleAdminRevealAuthorize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref    string `json:"ref"`
		Reason string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Ref) == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	entry, err := s.db.GetSecretCatalogByRef(req.Ref)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "secret catalog entry not found")
		return
	}
	now := time.Now().UTC()
	revealUntil := now.Add(adminRevealWindow)
	if err := s.db.UpdateAdminSessionRevealUntil(session.SessionID, &revealUntil); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to record reveal authorization")
		return
	}
	_ = s.db.MarkSecretCatalogRevealed(req.Ref, now)
	s.saveAuditEvent("secret", entry.RefCanonical, "admin_reveal_authorize", "admin_session", session.SessionID, strings.TrimSpace(req.Reason), "admin_auth", nil, map[string]any{
		"ref_canonical":  entry.RefCanonical,
		"vault_hash":     entry.VaultHash,
		"reveal_until":   revealUntil.Format(time.RFC3339),
		"authorized_at":  now.Format(time.RFC3339),
		"reason_present": strings.TrimSpace(req.Reason) != "",
	})
	s.respondJSON(w, http.StatusOK, map[string]any{
		"authorized":            true,
		"ref":                   entry.RefCanonical,
		"reveal_window_seconds": int(adminRevealWindow.Seconds()),
		"reveal_until":          revealUntil.Format(time.RFC3339),
		"policy":                "authorization recorded; plaintext reveal remains explicit and separately auditable",
	})
}

func (s *Server) handleAdminReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref string `json:"ref"`
	}
	if err := decodeRequestJSON(r, &req); err != nil || strings.TrimSpace(req.Ref) == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	now := time.Now().UTC()
	if session.RevealUntil == nil || now.After(session.RevealUntil.UTC()) {
		s.respondError(w, http.StatusForbidden, "reveal authorization required")
		return
	}
	payload, err := s.resolveAdminReveal(req.Ref)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = s.db.MarkSecretCatalogRevealed(payload["ref"].(string), now)
	s.saveAuditEvent("secret", payload["ref"].(string), "admin_reveal", "admin_session", session.SessionID, "", "admin_auth", nil, map[string]any{
		"ref":           payload["ref"],
		"name":          payload["name"],
		"vault":         payload["vault"],
		"revealed_at":   now.Format(time.RFC3339),
		"reveal_until":  session.RevealUntil.UTC().Format(time.RFC3339),
		"value_present": true,
	})
	payload["reveal_until"] = session.RevealUntil.UTC().Format(time.RFC3339)
	payload["revealed_at"] = now.Format(time.RFC3339)
	s.respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleAdminRebindApprovalsList(w http.ResponseWriter, r *http.Request) {
	agents, err := s.db.ListAgents()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	items := make([]map[string]any, 0)
	for _, agent := range agents {
		if !agent.RebindRequired && agent.BlockedAt == nil {
			continue
		}
		item := rebindApprovalPayload(&agent)
		latestEvidence, _ := s.db.GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "")
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
	s.respondJSON(w, http.StatusOK, map[string]any{
		"approvals": items,
		"count":     len(items),
	})
}

func (s *Server) handleAdminRebindPlan(w http.ResponseWriter, r *http.Request) {
	agent, err := s.findAgentRecord(r.PathValue("agent"))
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		s.respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}
	latestEvidence, _ := s.db.GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "submitted")
	payload := map[string]any{
		"status":                "plan",
		"vault_runtime_hash":    agent.AgentHash,
		"vault_node_uuid":       agent.NodeID,
		"vault_id":              formatVaultID(agent.VaultName, agent.VaultHash),
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
	s.respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleAdminApproveRebind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "approval payload is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	agent, err := s.findAgentRecord(r.PathValue("agent"))
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	requiredConfirm := "APPROVE " + strings.ToUpper(strings.TrimSpace(agent.AgentHash))
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != requiredConfirm {
		s.respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	approvalReason := strings.TrimSpace(req.Reason)
	if approvalReason == "" {
		s.respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		s.respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}
	evidence, err := s.db.GetLatestApprovalTokenChallenge(agent.AgentHash, "secure_input", "submitted")
	if err != nil || evidence == nil {
		s.respondError(w, http.StatusConflict, "submitted secure input evidence required")
		return
	}
	updated, err := s.db.ApproveAgentRebind(agent.NodeID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to approve agent rebind: "+err.Error())
		return
	}
	s.saveAuditEvent(
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
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":             "approved",
		"vault_runtime_hash": updated.AgentHash,
		"vault_node_uuid":    updated.NodeID,
		"vault_id":           formatVaultID(updated.VaultName, updated.VaultHash),
		"managed_paths":      db.DecodeManagedPaths(updated.ManagedPaths),
		"key_version":        updated.KeyVersion,
		"reason":             approvalReason,
		"evidence_token":     evidence.Token,
	})
}

func (s *Server) handleAdminScheduleAllRotations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "rotation payload is required")
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != "ROTATE ALL" {
		s.respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		s.respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	_, err := s.db.AdvancePendingRotations(time.Now().UTC())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to advance pending rotations: "+err.Error())
		return
	}
	agents, err := s.db.ScheduleAllAgentRotations(reason)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to schedule agent rotation: "+err.Error())
		return
	}
	results := make([]map[string]any, 0, len(agents))
	for _, agent := range agents {
		s.saveAuditEvent("vault", agent.NodeID, "admin_schedule_rotation", "admin_session", session.SessionID, reason, "admin_rotate_all", nil, map[string]any{
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rotation_required":  agent.RotationRequired,
			"reason":             reason,
		})
		results = append(results, map[string]any{
			"node_id":            agent.NodeID,
			"vault_node_uuid":    agent.NodeID,
			"label":              agent.Label,
			"vault_id":           formatVaultID(agent.VaultName, agent.VaultHash),
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rotation_required":  agent.RotationRequired,
		})
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status": "scheduled",
		"count":  len(results),
		"reason": reason,
		"agents": results,
	})
}

func (s *Server) handleAdminScheduleRotation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "rotation payload is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	agent, err := s.findAgentRecord(r.PathValue("agent"))
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	requiredConfirm := "ROTATE " + strings.ToUpper(strings.TrimSpace(agent.AgentHash))
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != requiredConfirm {
		s.respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		s.respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if agent.RotationRequired {
		s.respondError(w, http.StatusBadRequest, "rotation already scheduled")
		return
	}
	updated, err := s.db.ScheduleAgentRotation(agent.NodeID, reason)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to schedule agent rotation: "+err.Error())
		return
	}
	s.saveAuditEvent("vault", updated.NodeID, "admin_schedule_rotation_single", "admin_session", session.SessionID, reason, "admin_rotate_single", map[string]any{
		"vault_runtime_hash": agent.AgentHash,
		"key_version":        agent.KeyVersion,
		"rotation_required":  agent.RotationRequired,
	}, map[string]any{
		"vault_runtime_hash": updated.AgentHash,
		"key_version":        updated.KeyVersion,
		"rotation_required":  updated.RotationRequired,
		"reason":             reason,
	})
	s.respondJSON(w, http.StatusOK, map[string]any{
		"status":             "scheduled",
		"vault_runtime_hash": updated.AgentHash,
		"vault_node_uuid":    updated.NodeID,
		"vault_id":           formatVaultID(updated.VaultName, updated.VaultHash),
		"key_version":        updated.KeyVersion,
		"rotation_required":  updated.RotationRequired,
		"reason":             reason,
	})
}

func (s *Server) handleAdminTrackedRefCleanupPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reasons []string `json:"reasons"`
	}
	if r.Body != nil {
		_ = decodeJSON(r, &req)
	}
	resp, err := s.adminTrackedRefCleanupResponse(false, req.Reasons)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load tracked ref audit")
		return
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAdminTrackedRefCleanupApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm string   `json:"confirm"`
		Reason  string   `json:"reason"`
		Reasons []string `json:"reasons"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "cleanup payload is required")
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.Confirm)) != "CLEANUP STALE REFS" {
		s.respondError(w, http.StatusBadRequest, "confirmation text mismatch")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		s.respondError(w, http.StatusBadRequest, "reason is required")
		return
	}
	session, _ := s.currentAdminSession(r)
	resp, err := s.adminTrackedRefCleanupResponse(true, req.Reasons)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to apply tracked ref cleanup")
		return
	}
	s.saveAuditEvent("tracked_ref", "cleanup", "admin_cleanup_tracked_refs", "admin_session", session.SessionID, reason, "admin_tracked_ref_cleanup", nil, map[string]any{
		"counts":  resp.Counts,
		"actions": resp.Actions,
		"reason":  reason,
	})
	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) requireAdminSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.currentAdminSession(r); err != nil {
			s.respondError(w, http.StatusUnauthorized, "admin session required")
			return
		}
		next(w, r)
	}
}

func (s *Server) currentAdminSession(r *http.Request) (*db.AdminSession, error) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, fmt.Errorf("admin session cookie not found")
	}
	session, err := s.db.GetAdminSessionByTokenHash(hashAdminSessionToken(cookie.Value))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if session.RevokedAt != nil || now.After(session.ExpiresAt) || now.After(session.IdleExpiresAt) {
		if session.RevokedAt == nil {
			_ = s.db.RevokeAdminSession(session.SessionID, now)
		}
		return nil, fmt.Errorf("admin session expired")
	}
	session.LastSeenAt = now
	session.IdleExpiresAt = now.Add(adminSessionIdleTimeout)
	_ = s.db.TouchAdminSession(session.SessionID, session.LastSeenAt, session.IdleExpiresAt)
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
		SessionTTLSeconds:   int64(adminSessionTTL.Seconds()),
		IdleTTLSeconds:      int64(adminSessionIdleTimeout.Seconds()),
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
		"session_ttl_seconds":  int64(adminSessionTTL.Seconds()),
		"idle_timeout_seconds": int64(adminSessionIdleTimeout.Seconds()),
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

func (s *Server) encryptAdminSecret(secret string) ([]byte, []byte, error) {
	dek, err := s.getLocalDEK()
	if err != nil {
		return nil, nil, err
	}
	return crypto.Encrypt(dek, []byte(secret))
}

func (s *Server) decryptAdminSecret(ciphertext, nonce []byte) (string, error) {
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return "", nil
	}
	dek, err := s.getLocalDEK()
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
	v.Set("algorithm", "SHA1")
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func verifyTOTP(secret, code string, now time.Time) bool {
	normalized := strings.TrimSpace(code)
	if len(normalized) != adminTOTPDigits {
		return false
	}
	for offset := -1; offset <= 1; offset++ {
		if totpCode(secret, now.Add(time.Duration(offset*adminTOTPPeriodSeconds)*time.Second)) == normalized {
			return true
		}
	}
	return false
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
	mac := hmac.New(sha1.New, key)
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
	return decodeJSON(r, dst)
}

func (s *Server) resolveAdminReveal(ref string) (map[string]any, error) {
	tracked, err := s.db.GetRef(strings.TrimSpace(ref))
	if err != nil || tracked == nil {
		return nil, fmt.Errorf("tracked ref not found")
	}
	if tracked.AgentHash == "" {
		return nil, fmt.Errorf("tracked ref is not revealable")
	}
	agent, err := s.db.GetAgentByHash(tracked.AgentHash)
	if err != nil || len(agent.DEK) == 0 {
		return nil, fmt.Errorf("tracked ref vault is unavailable")
	}
	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to load vault key")
	}
	ai := agentToInfo(agent)
	cipher, err := s.fetchAgentCiphertext(ai.URL(), tracked.RefID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ciphertext")
	}
	plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret")
	}
	return map[string]any{
		"ref":                tracked.RefCanonical,
		"name":               cipher.Name,
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
		"vault_id":            formatVaultID(agent.VaultName, agent.VaultHash),
		"current_key_version": agent.KeyVersion,
		"next_key_version":    agent.KeyVersion + 1,
		"rebind_required":     agent.RebindRequired,
		"rebind_reason":       agent.RebindReason,
		"blocked":             agent.BlockedAt != nil,
		"block_reason":        agent.BlockReason,
		"managed_paths":       db.DecodeManagedPaths(agent.ManagedPaths),
	}
}

func (s *Server) adminTrackedRefCleanupResponse(apply bool, reasons []string) (*trackedRefCleanupResponse, error) {
	report, err := s.loadTrackedRefAuditReport()
	if err != nil {
		return nil, err
	}
	reasonFilter := make(map[string]bool, len(reasons))
	for _, reason := range reasons {
		if reason != "" {
			reasonFilter[reason] = true
		}
	}
	actions := buildTrackedRefCleanupActions(report, reasonFilter)
	resp := &trackedRefCleanupResponse{
		Status:  "preview",
		Apply:   apply,
		Actions: actions,
		Counts: map[string]int{
			"actions":           len(actions),
			"delete_candidates": 0,
			"manual_actions":    0,
			"deleted":           0,
		},
	}
	for _, action := range actions {
		resp.Counts["delete_candidates"] += len(action.Delete)
		if action.Manual {
			resp.Counts["manual_actions"]++
		}
	}
	if !apply {
		return resp, nil
	}
	resp.Status = "applied"
	for _, action := range actions {
		if action.Manual {
			continue
		}
		for _, ref := range action.Delete {
			if err := s.deleteTrackedRef(ref); err == nil {
				resp.Counts["deleted"]++
			}
		}
	}
	return resp, nil
}
