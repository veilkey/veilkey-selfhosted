package hkm

import (
	"net/http"

	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handleAgentGetSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")

	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	agentURL := agent.URL()
	meta, status, body, err := h.fetchAgentSecretMeta(agentURL, name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	if status != http.StatusOK {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.WriteHeader(status)
		w.Write(body)
		return
	}
	if meta == nil || meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope")
		return
	}
	_ = h.upsertTrackedRef(r.Context(), meta.Token, agent.KeyVersion, refStatus(meta.Status), agent.AgentHash)

	cipher, err := h.fetchAgentCiphertext(agentURL, meta.Ref)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to fetch ciphertext")
		return
	}
	plaintext, decErr := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
	if decErr != nil {
		respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}
	plaintextValue := string(plaintext)

	payload := map[string]interface{}{
		"name":         name,
		"value":        plaintextValue,
		"ref":          meta.Ref,
		"token":        meta.Token,
		"scope":        meta.Scope,
		"status":       meta.Status,
		"vault":        agent.Label,
		"fields":       meta.Fields,
		"fields_count": meta.FieldsCount,
	}
	setRuntimeHashAliases(payload, agent.AgentHash)
	respondJSON(w, http.StatusOK, payload)
}
