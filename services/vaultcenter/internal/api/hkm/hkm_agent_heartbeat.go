package hkm

import (
	"log"
	"net"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"strings"
	"time"
	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
)

func (h *Handler) preferredHeartbeatIP(r *http.Request) string {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if h.deps.IsTrustedIPString(clientIP) {
		for _, value := range []string{
			r.Header.Get("CF-Connecting-IP"),
			r.Header.Get("X-Real-IP"),
			r.Header.Get("X-Forwarded-For"),
		} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if strings.Contains(value, ",") {
				value = strings.TrimSpace(strings.Split(value, ",")[0])
			}
			if ip := net.ParseIP(value); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
			return ip.String()
		}
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if ip := net.ParseIP(remote); ip != nil {
		return ip.String()
	}
	return ""
}

func (h *Handler) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	h.advancePendingRotationsBestEffort()

	var req struct {
		VaultNodeUUID     string   `json:"vault_node_uuid"`
		NodeID            string   `json:"node_id"`
		Label             string   `json:"label"`
		VaultHash         string   `json:"vault_hash"`
		VaultName         string   `json:"vault_name"`
		Role              string   `json:"role"`
		LocalVaultRole    string   `json:"localvault_role"`
		HostEnabled       *bool    `json:"host_enabled"`
		LocalEnabled      *bool    `json:"local_enabled"`
		ManagedPaths      []string `json:"managed_paths"`
		KeyVersion        int      `json:"key_version"`
		IP                string   `json:"ip"`
		Port              int      `json:"port"`
		SecretsCount      int      `json:"secrets_count"`
		ConfigsCount      int      `json:"configs_count"`
		Version           int      `json:"version"`
		RegistrationToken string   `json:"registration_token"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nodeID := req.VaultNodeUUID
	if nodeID == "" {
		nodeID = req.NodeID
	}
	if nodeID == "" || req.Label == "" || req.VaultHash == "" || req.VaultName == "" {
		respondError(w, http.StatusBadRequest, "vault_node_uuid (or node_id), label, vault_hash, and vault_name are required")
		return
	}
	if req.Port < 0 || req.Port > 65535 {
		respondError(w, http.StatusBadRequest, "port must be between 0 and 65535")
		return
	}
	if req.KeyVersion == 0 {
		req.KeyVersion = 1
	}
	role := strings.TrimSpace(req.LocalVaultRole)
	if role == "" {
		role = strings.TrimSpace(req.Role)
	}
	roleProvided := role != ""
	if role == "" {
		role = "agent"
	}

	if req.IP == "" {
		req.IP = h.preferredHeartbeatIP(r)
	}

	agent, err := h.deps.DB().GetAgentByNodeID(nodeID)
	if err == nil {
		if agent.BlockedAt != nil && agent.BlockReason == "key_version_mismatch" && agent.KeyVersion == req.KeyVersion {
			agent, err = h.deps.DB().ClearAgentRebind(nodeID)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to clear blocked rebind state: "+err.Error())
				return
			}
		}
		if agent.BlockedAt != nil {
			respondJSON(w, http.StatusLocked, heartbeatAgentState(agent, "blocked"))
			return
		}
		if agent.RotationRequired {
			if agent.KeyVersion == req.KeyVersion {
				agent, err = h.deps.DB().ClearAgentRotation(nodeID)
				if err != nil {
					respondError(w, http.StatusInternalServerError, "failed to clear rotation state: "+err.Error())
					return
				}
			} else {
				resp := heartbeatAgentState(agent, "rotation_required")
				resp["expected_key_version"] = agent.KeyVersion
				resp["provided_key_version"] = req.KeyVersion
				respondJSON(w, http.StatusConflict, resp)
				return
			}
		}
		if agent.RebindRequired && agent.RebindReason == "key_version_mismatch" && agent.KeyVersion == req.KeyVersion {
			agent, err = h.deps.DB().ClearAgentRebind(nodeID)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to clear rebind state: "+err.Error())
				return
			}
		}
		if agent.RebindRequired {
			agent, err = h.deps.DB().AdvanceAgentRebind(nodeID, agent.RebindReason, time.Now().UTC())
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to update rebind state: "+err.Error())
				return
			}
			if agent.BlockedAt != nil {
				respondJSON(w, http.StatusLocked, heartbeatAgentState(agent, "blocked"))
				return
			}
			respondJSON(w, http.StatusConflict, heartbeatAgentState(agent, "rebind_required"))
			return
		}
		if agent.AgentHash != "" && agent.KeyVersion != 0 && agent.KeyVersion != req.KeyVersion {
			agent, err = h.deps.DB().AdvanceAgentRebind(nodeID, "key_version_mismatch", time.Now().UTC())
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to update rebind state: "+err.Error())
				return
			}
			if agent.BlockedAt != nil {
				resp := heartbeatAgentState(agent, "blocked")
				resp["expected_key_version"] = agent.KeyVersion
				resp["provided_key_version"] = req.KeyVersion
				respondJSON(w, http.StatusLocked, resp)
				return
			}
			resp := heartbeatAgentState(agent, "key_version_mismatch")
			resp["expected_key_version"] = agent.KeyVersion
			resp["provided_key_version"] = req.KeyVersion
			respondJSON(w, http.StatusConflict, resp)
			return
		}
	} else {
		agent = nil
	}

	// New agent registration requires a valid registration token
	if agent == nil {
		if req.RegistrationToken == "" {
			respondError(w, http.StatusForbidden, "registration_token is required for first-time agent registration")
			return
		}
		regToken, tokenErr := h.deps.DB().ValidateRegistrationToken(req.RegistrationToken)
		if tokenErr != nil || regToken == nil {
			respondError(w, http.StatusForbidden, "invalid or expired registration token")
			return
		}
		// Token will be consumed after successful registration (below)
	}

	if err := h.deps.DB().UpsertAgent(nodeID, req.Label, req.VaultHash, req.VaultName, req.IP, req.Port, req.SecretsCount, req.ConfigsCount, req.Version, req.KeyVersion); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to upsert agent: "+err.Error())
		return
	}
	if roleProvided || agent == nil {
		if err := h.deps.DB().UpdateAgentRole(nodeID, role); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update agent role: "+err.Error())
			return
		}
	}
	if req.HostEnabled != nil || req.LocalEnabled != nil || roleProvided || agent == nil {
		if err := h.deps.DB().UpdateAgentCapabilities(nodeID, role, req.HostEnabled, req.LocalEnabled); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update agent capabilities: "+err.Error())
			return
		}
	}
	if err := h.deps.DB().UpdateAgentManagedPaths(nodeID, req.ManagedPaths); err != nil {
		respondError(w, http.StatusConflict, "failed to update agent managed_paths: "+err.Error())
		return
	}

	agent, err = h.deps.DB().GetAgentByNodeID(nodeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get agent: "+err.Error())
		return
	}

	if agent.AgentHash == "" {
		agentHash, err := generateAgentHash()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate agent hash")
			return
		}

		agentDEK, err := crypto.GenerateKey()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate agent DEK")
			return
		}

		// kek lock handled by deps
		kek := h.deps.GetKEK()
		// kek unlock handled by deps

		encDEK, encNonce, err := crypto.Encrypt(kek, agentDEK)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to encrypt agent DEK")
			return
		}

		if err := h.deps.DB().UpdateAgentDEK(nodeID, agentHash, encDEK, encNonce); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to store agent DEK: "+err.Error())
			return
		}

		log.Printf("agent: assigned hash=%s to %s (%s)", agentHash, nodeID, req.Label)

		// Consume the registration token (one-time use)
		if req.RegistrationToken != "" {
			if err := h.deps.DB().ConsumeRegistrationToken(req.RegistrationToken, nodeID); err != nil {
				log.Printf("agent: failed to consume registration token %s: %v", req.RegistrationToken, err)
			}
		}

		resp := map[string]interface{}{
			"status":        "registered",
			"vault_id":      httputil.FormatVaultID(req.VaultName, req.VaultHash),
			"managed_paths": db.DecodeManagedPaths(agent.ManagedPaths),
			"key_version":   req.KeyVersion,
		}
		setNodeIdentityAliases(resp, nodeID)
		setRuntimeHashAliases(resp, agentHash)
		respondJSON(w, http.StatusOK, resp)
		return
	}

	resp := map[string]interface{}{
		"status":        "ok",
		"vault_id":      httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
		"managed_paths": db.DecodeManagedPaths(agent.ManagedPaths),
		"key_version":   agent.KeyVersion,
	}
	setNodeIdentityAliases(resp, nodeID)
	setRuntimeHashAliases(resp, agent.AgentHash)
	respondJSON(w, http.StatusOK, resp)
}

func heartbeatAgentState(agent *db.Agent, status string) map[string]interface{} {
	resp := map[string]interface{}{
		"status":            status,
		"vault_id":          httputil.FormatVaultID(agent.VaultName, agent.VaultHash),
		"managed_paths":     db.DecodeManagedPaths(agent.ManagedPaths),
		"key_version":       agent.KeyVersion,
		"rotation_required": agent.RotationRequired,
		"rebind_required":   agent.RebindRequired,
		"retry_stage":       agent.RetryStage,
	}
	setNodeIdentityAliases(resp, agent.NodeID)
	setRuntimeHashAliases(resp, agent.AgentHash)
	if agent.NextRetryAt != nil {
		retryAfter := int(time.Until(*agent.NextRetryAt).Seconds())
		if retryAfter < 0 {
			retryAfter = 0
		}
		resp["next_retry_at"] = agent.NextRetryAt.UTC().Format(time.RFC3339)
		resp["retry_after_seconds"] = retryAfter
	}
	if agent.RebindReason != "" {
		resp["rebind_reason"] = agent.RebindReason
	}
	if agent.RotationReason != "" {
		resp["rotation_reason"] = agent.RotationReason
	}
	if agent.BlockedAt != nil {
		resp["blocked"] = true
		resp["blocked_at"] = agent.BlockedAt.UTC().Format(time.RFC3339)
		resp["block_reason"] = agent.BlockReason
	} else {
		resp["blocked"] = false
	}
	return resp
}

