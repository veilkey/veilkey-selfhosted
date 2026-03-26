package secrets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handleSaveSecret(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusForbidden, vaultcenterOnlyDecryptMessage)
}

func (h *Handler) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusForbidden, vaultcenterOnlyDecryptMessage)
}

func (h *Handler) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := h.deps.DB().ListSecrets()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list secrets")
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
			Scope:   string(secret.Scope),
			Version: secret.Version,
			Status:  string(secret.Status),
		}
		if secret.Ref != "" {
			sr.Token = vkRef(secret.Scope, secret.Ref)
		}
		if fields, err := h.deps.DB().ListSecretFields(secret.Name); err == nil {
			sr.FieldsCount = len(fields)
		}
		result = append(result, sr)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"secrets": result,
		"count":   len(result),
	})
}

func (h *Handler) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "secret name is required")
		return
	}
	if !isValidResourceName(name) {
		respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	if err := h.deps.DB().DeleteSecret(name); err != nil {
		respondError(w, http.StatusNotFound, "secret not found")
		return
	}
	h.deps.DB().BumpContentVersion()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": name,
	})
}

func (h *Handler) handleResolveSecret(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusForbidden, "localvault direct plaintext resolution is disabled")
}

func (h *Handler) handleRekey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DEK     []byte `json:"dek"`
		Version int    `json:"version"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.DEK) != 32 {
		respondError(w, http.StatusBadRequest, "DEK must be 32 bytes")
		return
	}
	if req.Version <= 0 {
		respondError(w, http.StatusBadRequest, "version must be positive")
		return
	}

	oldDEK, err := h.deps.GetLocalDEK()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get current DEK")
		return
	}
	count, skipped, err := h.deps.DB().ReencryptMixedSecrets(
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
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("re-encryption failed after %d secrets (skipped %d already-current)", count, skipped))
		return
	}

	encDEK, encNonce, err := crypto.Encrypt(h.deps.GetKEK(), req.DEK)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to re-encrypt DEK with KEK")
		return
	}
	if err := h.deps.DB().UpdateNodeDEK(encDEK, encNonce, req.Version); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update node DEK")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "rekeyed",
		"secrets_updated": count,
		"secrets_skipped": skipped,
		"version":         req.Version,
	})
}

func (h *Handler) handleCipher(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	secret, err := h.deps.DB().GetSecretByRef(ref)
	if err != nil {
		respondError(w, http.StatusNotFound, "ref not found: "+ref)
		return
	}
	if secret.Status == refStatusBlock {
		respondError(w, http.StatusLocked, "ref is blocked: "+vkRef(secret.Scope, ref))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ref":        ref,
		"name":       secret.Name,
		"ciphertext": secret.Ciphertext,
		"nonce":      secret.Nonce,
		"version":    secret.Version,
	})
	_ = h.deps.DB().MarkSecretRevealed(ref, time.Now().UTC())
}
