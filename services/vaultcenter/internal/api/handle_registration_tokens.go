package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"veilkey-vaultcenter/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
)

type registrationTokenPayload struct {
	TokenID   string `json:"t"`
	URL       string `json:"u"`
	Label     string `json:"l,omitempty"`
	ExpiresAt int64  `json:"x"`
}

func encodeRegistrationToken(tokenID, vcURL, label string, expiresAt time.Time) string {
	payload, _ := json.Marshal(registrationTokenPayload{
		TokenID:   tokenID,
		URL:       vcURL,
		Label:     label,
		ExpiresAt: expiresAt.Unix(),
	})
	return "vk_reg_" + base64.RawURLEncoding.EncodeToString(payload)
}

func (s *Server) handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label            string `json:"label"`
		ExpiresInMinutes int    `json:"expires_in_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ExpiresInMinutes <= 0 {
		req.ExpiresInMinutes = 60 // default 1 hour
	}
	if req.ExpiresInMinutes > 10080 { // max 7 days
		req.ExpiresInMinutes = 10080
	}

	tokenID := crypto.GenerateUUID()
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInMinutes) * time.Minute)

	token := &db.RegistrationToken{
		TokenID:   tokenID,
		Label:     req.Label,
		CreatedBy: "admin",
		Status:    "active",
		ExpiresAt: expiresAt,
	}
	if err := s.db.SaveRegistrationToken(token); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to create registration token")
		return
	}

	// Determine public VC URL
	vcURL := ""
	if uiConfig, err := s.db.GetUIConfig(); err == nil && uiConfig.VaultcenterURL != "" {
		vcURL = uiConfig.VaultcenterURL
	}

	encoded := encodeRegistrationToken(tokenID, vcURL, req.Label, expiresAt)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"token_id":   tokenID,
		"token":      encoded,
		"label":      req.Label,
		"status":     "active",
		"expires_at": expiresAt,
		"command":    "veilkey-localvault init --root --token " + encoded,
	})
}

func (s *Server) handleListRegistrationTokens(w http.ResponseWriter, r *http.Request) {
	tokens, total, err := s.db.ListRegistrationTokens(50, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list registration tokens")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"tokens": tokens,
		"total":  total,
	})
}

func (s *Server) handleRevokeRegistrationToken(w http.ResponseWriter, r *http.Request) {
	tokenID := r.PathValue("token_id")
	if tokenID == "" {
		s.respondError(w, http.StatusBadRequest, "token_id is required")
		return
	}
	if err := s.db.RevokeRegistrationToken(tokenID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
}

func (s *Server) handleValidateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	tokenID := r.PathValue("token_id")
	if tokenID == "" {
		s.respondError(w, http.StatusBadRequest, "token_id is required")
		return
	}
	token, err := s.db.ValidateRegistrationToken(tokenID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "token not found or expired")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"valid":      true,
		"label":      token.Label,
		"expires_at": token.ExpiresAt,
	})
}
