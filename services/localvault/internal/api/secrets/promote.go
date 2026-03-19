package secrets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handlePromote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref  string `json:"ref"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ref == "" || req.Name == "" {
		respondError(w, http.StatusBadRequest, "ref and name are required")
		return
	}
	if !strings.HasPrefix(req.Ref, makeRef(refFamilyVK, refScopeTemp, "")) {
		respondError(w, http.StatusBadRequest, "only VK:TEMP refs can be promoted")
		return
	}
	if !isValidResourceName(req.Name) {
		respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	vcURL := h.deps.VaultcenterURL()
	if vcURL == "" {
		respondError(w, http.StatusServiceUnavailable, "vaultcenter URL not configured")
		return
	}

	resolveURL := vcURL + "/api/resolve/" + url.PathEscape(req.Ref)
	resp, err := h.deps.HTTPClient().Get(resolveURL)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to reach vaultcenter: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		respondError(w, http.StatusBadGateway, fmt.Sprintf("vaultcenter resolve failed (%d): %s", resp.StatusCode, string(body)))
		return
	}

	var resolveResp struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resolveResp); err != nil {
		respondError(w, http.StatusBadGateway, "failed to parse vaultcenter response")
		return
	}
	if resolveResp.Value == "" {
		respondError(w, http.StatusBadGateway, "vaultcenter returned empty value")
		return
	}

	nodeInfo, err := h.deps.DB().GetNodeInfo()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}

	dek, err := crypto.Decrypt(h.deps.GetKEK(), nodeInfo.DEK, nodeInfo.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt DEK")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(dek, []byte(resolveResp.Value))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	refID, err := generateSecretRef(8)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate ref")
		return
	}

	secret := &db.Secret{
		ID:         crypto.GenerateUUID(),
		Name:       strings.ToUpper(req.Name),
		Ref:        refID,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Version:    nodeInfo.Version,
		Scope:      refScopeLocal,
		Status:     refStatusActive,
	}
	if err := h.deps.DB().SaveSecret(secret); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save secret: "+err.Error())
		return
	}

	token := vkRef(refScopeLocal, refID)
	respondJSON(w, http.StatusOK, map[string]any{
		"ref":    token,
		"token":  token,
		"name":   secret.Name,
		"status": refStatusActive,
		"action": "promoted",
	})
}
