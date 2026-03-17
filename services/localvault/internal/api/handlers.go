package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"veilkey-localvault/internal/crypto"
)

func supportedFeatures() []string {
	return []string{
		"status",
		"node_info",
		"secrets",
		"configs",
		"resolve",
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if s.IsLocked() {
		status = "locked"
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		s.respondError(w, http.StatusServiceUnavailable, "server is locked")
		return
	}
	if err := s.db.Ping(); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	secretCount, _ := s.db.CountSecrets()
	configCount, _ := s.db.CountConfigs()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"mode":               "vault",
		"node_id":            info.NodeID,
		"vault_node_uuid":    info.NodeID,
		"vault_hash":         s.identity.VaultHash,
		"vault_name":         s.identity.VaultName,
		"vault_id":           formatVaultID(s.identity.VaultName, s.identity.VaultHash),
		"version":            info.Version,
		"secrets_count":      secretCount,
		"configs_count":      configCount,
		"locked":             s.IsLocked(),
		"supported_features": supportedFeatures(),
	})
}

func formatVaultID(name, hash string) string {
	if hash == "" {
		return name
	}
	if name == "" {
		return hash
	}
	return name + ":" + hash
}

func (s *Server) handleSaveSecret(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, http.StatusForbidden, keycenterOnlyDecryptMessage)
}

func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, http.StatusForbidden, keycenterOnlyDecryptMessage)
}

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := s.db.ListSecrets()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list secrets")
		return
	}

	type secretResp struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Ref         string `json:"ref,omitempty"`
		Token       string `json:"token,omitempty"`
		Scope       string `json:"scope"`
		Version     int    `json:"version"`
		Status      string `json:"status"`
		FieldsCount int    `json:"fields_count,omitempty"`
	}
	var result []secretResp
	for _, secret := range secrets {
		sr := secretResp{
			ID:      secret.ID,
			Name:    secret.Name,
			Ref:     secret.Ref,
			Scope:   secret.Scope,
			Version: secret.Version,
			Status:  secret.Status,
		}
		if secret.Ref != "" {
			sr.Token = ParsedRef{Family: RefFamilyVK, Scope: RefScope(secret.Scope), ID: secret.Ref}.CanonicalString()
		}
		fields, err := s.db.ListSecretFields(secret.Name)
		if err == nil {
			sr.FieldsCount = len(fields)
		}
		result = append(result, sr)
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"secrets": result,
		"count":   len(result),
	})
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "secret name is required")
		return
	}
	if !isValidResourceName(name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	if err := s.db.DeleteSecret(name); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": name,
	})
}

func (s *Server) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	// Only allow cascade requests from keycenter (federated resolve)
	if r.Header.Get("X-VeilKey-Cascade") != "true" {
		s.respondError(w, http.StatusForbidden, keycenterOnlyDecryptMessage)
		return
	}

	ref := r.PathValue("ref")
	if ref == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	secret, err := s.db.GetSecretByRef(ref)
	if err != nil {
		// Try by canonical ref parts
		parts := strings.SplitN(ref, ":", 3)
		if len(parts) == 3 {
			secret, err = s.db.GetSecretByRef(parts[2])
		}
		if err != nil {
			s.respondError(w, http.StatusNotFound, "ref not found")
			return
		}
	}

	info, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()

	dek, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt DEK")
		return
	}

	plaintext, err := crypto.Decrypt(dek, secret.Ciphertext, secret.Nonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"ref":   ref,
		"name":  secret.Name,
		"value": string(plaintext),
	})
}

func (s *Server) handleRekey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DEK     []byte `json:"dek"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.DEK) != 32 {
		s.respondError(w, http.StatusBadRequest, "DEK must be 32 bytes")
		return
	}
	if req.Version <= 0 {
		s.respondError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	oldDEK, err := s.getLocalDEK()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get current DEK: "+err.Error())
		return
	}
	count, skipped, err := s.db.ReencryptMixedSecrets(
		func(ciphertext, nonce []byte) ([]byte, error) {
			return crypto.Decrypt(oldDEK, ciphertext, nonce)
		},
		func(ciphertext, nonce []byte) ([]byte, error) {
			return crypto.Decrypt(req.DEK, ciphertext, nonce)
		},
		func(plaintext []byte) ([]byte, []byte, error) {
			return crypto.Encrypt(req.DEK, plaintext)
		},
		req.Version,
	)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("re-encryption failed after %d secrets (skipped %d already-current): %v", count, skipped, err))
		return
	}

	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()
	encDEK, encNonce, err := crypto.Encrypt(kek, req.DEK)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to re-encrypt DEK with KEK")
		return
	}
	if err := s.db.UpdateNodeDEK(encDEK, encNonce, req.Version); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to update node DEK: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "rekeyed",
		"secrets_updated": count,
		"secrets_skipped": skipped,
		"version":         req.Version,
	})
}

// handleCipher returns raw encrypted secret (ciphertext + nonce) for Hub-only decryption
func (s *Server) handleCipher(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		s.respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	secret, err := s.db.GetSecretByRef(ref)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "ref not found: "+ref)
		return
	}
	if secret.Status == "block" {
		s.respondError(w, http.StatusLocked, "ref is blocked: VK:"+secret.Scope+":"+ref)
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"ref":        ref,
		"name":       secret.Name,
		"ciphertext": secret.Ciphertext,
		"nonce":      secret.Nonce,
		"version":    secret.Version,
	})
	_ = s.db.MarkSecretRevealed(ref, time.Now().UTC())
}

func generateSecretRef(length int) (string, error) {
	bytes := make([]byte, (length+1)/2)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func (s *Server) getLocalDEK() ([]byte, error) {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		return nil, fmt.Errorf("no node info: %w", err)
	}

	s.kekMu.RLock()
	kek := s.kek
	s.kekMu.RUnlock()

	dek, err := crypto.Decrypt(kek, info.DEK, info.DEKNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}
	return dek, nil
}
