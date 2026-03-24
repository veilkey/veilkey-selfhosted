package hkm

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// unlockKeyLimiter tracks per-agent request timestamps to enforce rate limiting
// on the unlock-key endpoint (max 5 requests per minute per agent).
var unlockKeyLimiter = struct {
	mu      sync.Mutex
	windows map[string][]time.Time // agentHash -> list of request timestamps
}{
	windows: make(map[string][]time.Time),
}

const (
	unlockKeyRateLimit  = 5
	unlockKeyRateWindow = time.Minute
)

// checkUnlockKeyRateLimit returns true if the agent is within the rate limit.
func checkUnlockKeyRateLimit(agentHash string) bool {
	unlockKeyLimiter.mu.Lock()
	defer unlockKeyLimiter.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-unlockKeyRateWindow)

	// Prune expired entries
	timestamps := unlockKeyLimiter.windows[agentHash]
	valid := timestamps[:0]
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= unlockKeyRateLimit {
		unlockKeyLimiter.windows[agentHash] = valid
		return false
	}

	unlockKeyLimiter.windows[agentHash] = append(valid, now)
	return true
}

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

	if !checkUnlockKeyRateLimit(authedHash) {
		log.Printf("agent: unlock-key rate limited for %s", authedHash)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(unlockKeyRateWindow.Seconds())))
		respondError(w, http.StatusTooManyRequests, "too many unlock-key requests, retry later")
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
