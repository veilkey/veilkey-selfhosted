package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentRebindPlan(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgentRecord(hashOrLabel)
	if err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":              "plan",
		"vault_runtime_hash":  agent.AgentHash,
		"vault_id":            httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
		"current_key_version": agent.KeyVersion,
		"next_key_version":    agent.KeyVersion + 1,
		"managed_paths":       db.DecodeManagedPaths(agent.ManagedPaths),
		"rebind_required":     agent.RebindRequired,
		"blocked":             agent.BlockedAt != nil,
	})
}
