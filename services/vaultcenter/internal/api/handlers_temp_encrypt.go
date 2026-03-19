package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/refs"
	"veilkey-vaultcenter/internal/api/hkm"
	"veilkey-vaultcenter/internal/db"
)

const tempKeyTTL = 1 * time.Hour

func (s *Server) handleTempEncrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plaintext string `json:"plaintext"`
		Name      string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Plaintext == "" {
		s.respondError(w, http.StatusBadRequest, "plaintext is required")
		return
	}

	// Check for existing active temp ref with same plaintext
	hash := sha256.Sum256([]byte(req.Plaintext))
	plaintextHash := hex.EncodeToString(hash[:])

	if existing, err := s.db.FindActiveTempRefByHash(plaintextHash); err == nil && existing != nil {
		resp := map[string]any{
			"ref":   existing.RefCanonical,
			"token": existing.RefCanonical,
		}
		if existing.ExpiresAt != nil {
			resp["expires_at"] = existing.ExpiresAt.Format(time.RFC3339)
		}
		if existing.SecretName != "" && existing.SecretName != existing.RefID {
			resp["name"] = existing.SecretName
		}
		s.respondJSON(w, http.StatusOK, resp)
		return
	}

	dek, err := s.GetLocalDEK()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get encryption key")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(dek, []byte(req.Plaintext))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	refID, err := hkm.GenerateSecretRef(16)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	parts := db.RefParts{Family: db.RefFamilyVK, Scope: db.RefScopeTemp, ID: refID}
	encoded := crypto.EncodeCiphertext(ciphertext, nonce)
	expiresAt := time.Now().UTC().Add(tempKeyTTL)

	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	name := strings.TrimSpace(req.Name)
	if _, err := s.SubmitTx(r.Context(), chain.TxSaveTokenRef, chain.SaveTokenRefPayload{
		RefFamily:     parts.Family,
		RefScope:      refs.RefScope(parts.Scope),
		RefID:         parts.ID,
		SecretName:    name,
		PlaintextHash: plaintextHash,
		Ciphertext:    encoded,
		Version:       nodeInfo.Version,
		Status:        refs.RefStatus(db.RefStatusTemp),
		ExpiresAt:     &expiresAt,
	}); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save temp ref")
		return
	}

	canonical := parts.Canonical()
	s.saveAuditEvent("token_ref", canonical, "temp_encrypt", "api", r.RemoteAddr, "", "api", nil, map[string]any{
		"ref":        canonical,
		"name":       name,
		"expires_at": expiresAt.Format(time.RFC3339),
	})

	resp := map[string]any{
		"ref":        canonical,
		"token":      canonical,
		"expires_at": expiresAt.Format(time.RFC3339),
	}
	if name != "" {
		resp["name"] = name
	}
	s.respondJSON(w, http.StatusOK, resp)
}
