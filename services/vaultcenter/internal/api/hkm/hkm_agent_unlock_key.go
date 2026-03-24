package hkm

import (
	"log"
	"net/http"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// handleAgentUnlockKey returns the vault unlock key for the authenticated agent.
// The agent authenticates via Bearer agent_secret. VaultCenter decrypts the stored
// vault_unlock_key with its KEK and returns it so the LocalVault can auto-unlock.
func (h *Handler) handleAgentUnlockKey(w http.ResponseWriter, r *http.Request) {
	// Agent is already authenticated by requireAgentAuth middleware.
	authedHash, ok := r.Context().Value(agentAuthKey).(string)
	if !ok || authedHash == "" {
		respondError(w, http.StatusUnauthorized, "agent authentication required")
		return
	}

	agent, err := h.deps.DB().GetAgentByHash(authedHash)
	if err != nil {
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}

	if len(agent.VaultUnlockKeyEnc) == 0 {
		respondError(w, http.StatusNotFound, "no vault unlock key stored for this agent")
		return
	}

	kek := h.deps.GetKEK()
	unlockKey, err := crypto.Decrypt(kek, agent.VaultUnlockKeyEnc, agent.VaultUnlockKeyNonce)
	if err != nil {
		log.Printf("agent: failed to decrypt vault_unlock_key for %s: %v", agent.NodeID, err)
		respondError(w, http.StatusInternalServerError, "failed to decrypt vault unlock key")
		return
	}

	log.Printf("agent: vault_unlock_key served for %s (%s)", agent.NodeID, agent.Label)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"unlock_key": string(unlockKey),
	})
}
