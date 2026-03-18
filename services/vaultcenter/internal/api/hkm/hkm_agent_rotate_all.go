package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"time"
)

func (h *Handler) handleAgentRotateAll(w http.ResponseWriter, r *http.Request) {
	reason := "planned_rotation"
	_, err := h.deps.DB().AdvancePendingRotations(time.Now().UTC())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to advance pending rotations: "+err.Error())
		return
	}
	agents, err := h.deps.DB().ScheduleAllAgentRotations(reason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to schedule agent rotation: "+err.Error())
		return
	}
	results := make([]map[string]interface{}, 0, len(agents))
	for _, agent := range agents {
		h.deps.SaveAuditEvent(
			"vault",
			agent.NodeID,
			"schedule_rotation",
			"operator",
			httputil.ActorIDForRequest(r),
			reason,
			"agent_rotate_all",
			nil,
			map[string]any{
				"vault_runtime_hash": agent.AgentHash,
				"key_version":        agent.KeyVersion,
				"rotation_required":  agent.RotationRequired,
			},
		)
		results = append(results, map[string]interface{}{
			"node_id":            agent.NodeID,
			"vault_node_uuid":    agent.NodeID,
			"label":              agent.Label,
			"vault_id":           httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
			"vault_runtime_hash": agent.AgentHash,
			"key_version":        agent.KeyVersion,
			"rotation_required":  agent.RotationRequired,
		})
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "scheduled",
		"count":  len(results),
		"agents": results,
	})
}
