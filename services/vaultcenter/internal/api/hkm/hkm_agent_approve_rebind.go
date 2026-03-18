package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentApproveRebind(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgentRecord(hashOrLabel)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if !agent.RebindRequired && agent.BlockedAt == nil {
		respondError(w, http.StatusBadRequest, "agent does not require rebind")
		return
	}

	updated, err := h.deps.DB().ApproveAgentRebind(agent.NodeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to approve agent rebind: "+err.Error())
		return
	}
	h.deps.SaveAuditEvent(
		"vault",
		updated.NodeID,
		"approve_rebind",
		"operator",
		httputil.ActorIDForRequest(r),
		"",
		"agent_approve_rebind",
		map[string]any{
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rebind_required":    agent.RebindRequired,
		},
		map[string]any{
			"vault_runtime_hash": updated.AgentHash,
			"key_version":        updated.KeyVersion,
			"rebind_required":    updated.RebindRequired,
		},
	)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":             "approved",
		"vault_runtime_hash": updated.AgentHash,
		"vault_id":           httputil.FormatVaultID(updated.VaultName, updated.VaultHash),
		"managed_paths":      db.DecodeManagedPaths(updated.ManagedPaths),
		"key_version":        updated.KeyVersion,
	})
}
