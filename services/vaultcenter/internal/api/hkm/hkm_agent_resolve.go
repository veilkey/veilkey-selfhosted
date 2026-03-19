package hkm

import (
	"net/http"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handleAgentResolve(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		respondError(w, http.StatusBadRequest, "token is required")
		return
	}

	if len(token) <= 8 {
		respondError(w, http.StatusBadRequest, "invalid token format: too short")
		return
	}

	agentHash := token[:8]
	secretRef := token[8:]

	agent, err := h.deps.DB().GetAgentByHash(agentHash)
	if err != nil {
		respondError(w, http.StatusNotFound, "agent not found for hash: "+agentHash)
		return
	}
	if err := validateAgentAvailability(agent); err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	ai := agentToInfo(agent)
	cipherSecret, err := h.fetchAgentCiphertext(ai.URL(), secretRef)
	if err != nil {
		respondError(w, http.StatusNotFound, "failed to fetch secret from agent")
		return
	}

	plaintext, err := crypto.Decrypt(agentDEK, cipherSecret.Ciphertext, cipherSecret.Nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	resp := map[string]interface{}{
		"ref":   secretRef,
		"vault": agent.Label,
		"name":  cipherSecret.Name,
		"value": string(plaintext),
	}
	setRuntimeHashAliases(resp, agentHash)
	respondJSON(w, http.StatusOK, resp)
}
