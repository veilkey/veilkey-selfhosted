package hkm

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// ── SSH key management endpoints ─────────────────────────────────

type sshKeyCreateRequest struct {
	Label      string `json:"label"`
	KeyType    string `json:"key_type"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

type sshKeyHostsRequest struct {
	HostsJSON string `json:"hosts_json"`
}

func (h *Handler) handleSSHKeysCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleSSHKeyCreate(w, r)
	case http.MethodGet:
		h.handleSSHKeyList(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSSHKeyCreate(w http.ResponseWriter, r *http.Request) {
	var req sshKeyCreateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if strings.TrimSpace(req.PublicKey) == "" {
		respondError(w, http.StatusBadRequest, "public_key is required")
		return
	}
	if strings.TrimSpace(req.KeyType) == "" {
		respondError(w, http.StatusBadRequest, "key_type is required")
		return
	}

	// Compute fingerprint from public key (SHA-256)
	fpHash := sha256.Sum256([]byte(req.PublicKey))
	fingerprint := hex.EncodeToString(fpHash[:])

	// Generate ref: VK:SSH:{first 8 chars of SHA-256(fingerprint)}
	refHash := sha256.Sum256([]byte(fingerprint))
	refID := hex.EncodeToString(refHash[:])[:8]
	ref := "VK:SSH:" + refID

	// Encrypt private key with KEK if provided
	var privEnc, privNonce []byte
	ownership := "external"
	if strings.TrimSpace(req.PrivateKey) != "" {
		kek := h.deps.GetKEK()
		if len(kek) == 0 {
			respondError(w, http.StatusServiceUnavailable, "server is locked")
			return
		}
		var err error
		privEnc, privNonce, err = crypto.Encrypt(kek, []byte(req.PrivateKey))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to encrypt private key")
			return
		}
		ownership = "own"
	}

	sshKey := db.SSHKey{
		Ref:             ref,
		Ownership:       ownership,
		Label:           req.Label,
		KeyType:         req.KeyType,
		Fingerprint:     fingerprint,
		PrivateKeyEnc:   privEnc,
		PrivateKeyNonce: privNonce,
		PublicKey:        req.PublicKey,
		HostsJSON:       "[]",
		MetadataJSON:    "{}",
	}

	if err := h.deps.DB().CreateSSHKey(&sshKey); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			respondError(w, http.StatusConflict, "ssh key with this fingerprint already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create ssh key")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"ref":         sshKey.Ref,
		"label":       sshKey.Label,
		"key_type":    sshKey.KeyType,
		"fingerprint": sshKey.Fingerprint,
		"ownership":   sshKey.Ownership,
		"public_key":  sshKey.PublicKey,
	})
}

func (h *Handler) handleSSHKeyList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.deps.DB().ListSSHKeys()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list ssh keys")
		return
	}

	// Return public info only (private key fields are json:"-" in model)
	respondJSON(w, http.StatusOK, map[string]any{
		"keys":  keys,
		"count": len(keys),
	})
}

func (h *Handler) handleSSHKeyByRef(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		key, err := h.deps.DB().GetSSHKey(ref)
		if err != nil {
			respondError(w, http.StatusNotFound, "ssh key not found")
			return
		}
		respondJSON(w, http.StatusOK, key)
	case http.MethodDelete:
		if err := h.deps.DB().DeleteSSHKey(ref); err != nil {
			respondError(w, http.StatusNotFound, "ssh key not found")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"deleted": ref})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSSHKeyDecrypt(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	key, err := h.deps.DB().GetSSHKey(ref)
	if err != nil {
		respondError(w, http.StatusNotFound, "ssh key not found")
		return
	}

	if len(key.PrivateKeyEnc) == 0 {
		respondError(w, http.StatusBadRequest, "no private key stored (external key)")
		return
	}

	kek := h.deps.GetKEK()
	if len(kek) == 0 {
		respondError(w, http.StatusServiceUnavailable, "server is locked")
		return
	}

	plaintext, err := crypto.Decrypt(kek, key.PrivateKeyEnc, key.PrivateKeyNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt private key")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"ref":         key.Ref,
		"private_key": string(plaintext),
	})
}

func (h *Handler) handleSSHKeyHosts(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	var req sshKeyHostsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if err := h.deps.DB().UpdateSSHKeyHosts(ref, req.HostsJSON); err != nil {
		respondError(w, http.StatusNotFound, "ssh key not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"updated": ref})
}
