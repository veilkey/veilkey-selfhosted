package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/crypto"
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

	dek, err := s.getLocalDEK()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get encryption key")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(dek, []byte(req.Plaintext))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	refID, err := generateSecretRef(16)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	parts := db.RefParts{Family: "VK", Scope: "TEMP", ID: refID}
	encoded := base64.StdEncoding.EncodeToString(ciphertext) + ":" + base64.StdEncoding.EncodeToString(nonce)
	expiresAt := time.Now().UTC().Add(tempKeyTTL)

	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	name := strings.TrimSpace(req.Name)
	if err := s.db.SaveRefWithExpiryAndHash(parts, encoded, nodeInfo.Version, "temp", expiresAt, name, plaintextHash); err != nil {
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
