package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"time"
	"veilkey-vaultcenter/internal/db"
)

func (h *Handler) handleAgentList(w http.ResponseWriter, r *http.Request) {
	h.advancePendingRotationsBestEffort()

	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	type agentResp struct {
		NodeID           string   `json:"node_id"`
		VaultNodeUUID    string   `json:"vault_node_uuid"`
		Label            string   `json:"label"`
		VaultRuntimeHash string   `json:"vault_runtime_hash"`
		AgentHash        string   `json:"agent_hash"`
		VaultHash        string   `json:"vault_hash"`
		VaultName        string   `json:"vault_name"`
		VaultID          string   `json:"vault_id"`
		ManagedPaths     []string `json:"managed_paths"`
		KeyVersion       int      `json:"key_version"`
		Status           string   `json:"status"`
		RotationRequired bool     `json:"rotation_required"`
		RebindRequired   bool     `json:"rebind_required"`
		RetryStage       int      `json:"retry_stage"`
		NextRetryAt      string   `json:"next_retry_at,omitempty"`
		Blocked          bool     `json:"blocked"`
		BlockReason      string   `json:"block_reason,omitempty"`
		IP               string   `json:"ip"`
		Port             int      `json:"port"`
		SecretsCount     int      `json:"secrets_count"`
		ConfigsCount     int      `json:"configs_count"`
		Version          int      `json:"version"`
		HasDEK           bool     `json:"has_dek"`
		LastSeen         string   `json:"last_seen"`
	}

	var result []agentResp
	for _, a := range agents {
		status := "ok"
		if a.BlockedAt != nil {
			status = "blocked"
		} else if a.RotationRequired {
			status = "rotation_required"
		} else if a.RebindRequired {
			status = "rebind_required"
		}
		nextRetryAt := ""
		if a.NextRetryAt != nil {
			nextRetryAt = a.NextRetryAt.UTC().Format(time.RFC3339)
		}
		result = append(result, agentResp{
			NodeID:           a.NodeID,
			VaultNodeUUID:    a.NodeID,
			Label:            a.Label,
			VaultRuntimeHash: a.AgentHash,
			AgentHash:        a.AgentHash,
			VaultHash:        a.VaultHash,
			VaultName:        a.VaultName,
			VaultID:          httputil.FormatVaultID(a.VaultName, a.VaultHash),
			ManagedPaths:     db.DecodeManagedPaths(a.ManagedPaths),
			KeyVersion:       a.KeyVersion,
			Status:           status,
			RotationRequired: a.RotationRequired,
			RebindRequired:   a.RebindRequired,
			RetryStage:       a.RetryStage,
			NextRetryAt:      nextRetryAt,
			Blocked:          a.BlockedAt != nil,
			BlockReason:      a.BlockReason,
			IP:               a.IP,
			Port:             a.Port,
			SecretsCount:     a.SecretsCount,
			ConfigsCount:     a.ConfigsCount,
			Version:          a.Version,
			HasDEK:           len(a.DEK) > 0,
			LastSeen:         a.LastSeen.Format("2006-01-02 15:04:05"),
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"agents": result,
		"count":  len(result),
	})
}
