package hkm

import (
	"fmt"
	"log"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// handleRekey accepts a new DEK from parent and re-encrypts all local secrets
func (h *Handler) handleRekey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DEK     []byte `json:"dek"`
		Version int    `json:"version"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
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

	oldDEK, err := h.getLocalDEK()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get current DEK")
		return
	}

	newDEK := req.DEK

	count, err := h.deps.DB().ReencryptAllSecrets(
		func(ciphertext, nonce []byte) ([]byte, error) {
			return crypto.Decrypt(oldDEK, ciphertext, nonce)
		},
		func(plaintext []byte) ([]byte, []byte, error) {
			return crypto.Encrypt(newDEK, plaintext)
		},
		req.Version,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("re-encryption failed after %d secrets: %v", count, err))
		return
	}

	// kek lock handled by deps
	kek := h.deps.GetKEK()
	// kek unlock handled by deps

	encDEK, encNonce, err := crypto.Encrypt(kek, newDEK)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to re-encrypt DEK with KEK")
		return
	}
	if err := h.deps.DB().UpdateNodeDEK(encDEK, encNonce, req.Version); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update node DEK: "+err.Error())
		return
	}

	log.Printf("Rekey complete: %d secrets re-encrypted to version %d", count, req.Version)

	childrenFailed := 0
	childrenUpdated := 0
	children, _ := h.deps.DB().ListChildren()
	for i := range children {
		child := &children[i]
		newChildDEK, err := crypto.GenerateKey()
		if err != nil {
			childrenFailed++
			continue
		}
		encChildDEK, childNonce, err := crypto.Encrypt(newDEK, newChildDEK)
		if err != nil {
			childrenFailed++
			continue
		}
		newChildVersion := child.Version + 1
		if err := h.deps.DB().UpdateChildDEK(child.NodeID, encChildDEK, childNonce, newChildVersion); err != nil {
			childrenFailed++
			continue
		}
		childrenUpdated++
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "rekeyed",
		"secrets_updated":  count,
		"version":          req.Version,
		"children_updated": childrenUpdated,
		"children_failed":  childrenFailed,
	})
}
