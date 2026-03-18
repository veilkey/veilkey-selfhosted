package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handleAgentSaveSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil || req.Name == "" || req.Value == "" {
		respondError(w, http.StatusBadRequest, "name and value are required")
		return
	}
	if !isValidResourceName(req.Name) {
		respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(agentDEK, []byte(req.Value))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to encrypt agent secret")
		return
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":       req.Name,
		"ciphertext": ciphertext,
		"nonce":      nonce,
		"version":    0,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}
	resp, err := h.deps.HTTPClient().Post(agent.URL()+agentPathCipher, "application/json", bytes.NewReader(body))
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}

	var data map[string]interface{}
	if json.Unmarshal(respBody, &data) == nil {
		if ref, ok := data["ref"].(string); ok && ref != "" {
			scopeStr, _ := data["scope"].(string)
			statusStr, _ := data["status"].(string)
			normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVK, refScope(scopeStr), refStatus(statusStr), refScopeTemp)
			if normalizeErr != nil {
				respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+normalizeErr.Error())
				return
			}
			canonical := "VK:" + string(normScope) + ":" + ref
			data["token"] = canonical
			data["scope"] = string(normScope)
			data["status"] = string(normStatus)
			_ = h.upsertTrackedRefNamed(canonical, agent.KeyVersion, normStatus, agent.AgentHash, req.Name)
			h.deps.SaveAuditEvent(
				"secret",
				canonical,
				"save",
				"agent",
				agent.AgentHash,
				"",
				"agent_save_secret",
				nil,
				map[string]any{
					"name":               req.Name,
					"ref":                canonical,
					"vault_runtime_hash": agent.AgentHash,
					"status":             string(normStatus),
				},
			)
		}
		data["vault"] = agent.Label
		setRuntimeHashAliases(data, agent.AgentHash)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
