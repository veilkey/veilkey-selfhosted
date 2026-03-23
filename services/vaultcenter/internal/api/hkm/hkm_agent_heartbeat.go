package hkm

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) preferredHeartbeatIP(r *http.Request) string {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	if h.deps.IsTrustedIPString(clientIP) {
		for _, value := range []string{
			r.Header.Get(httputil.HeaderCFConnectingIP),
			r.Header.Get(httputil.HeaderXRealIP),
			r.Header.Get(httputil.HeaderXForwardedFor),
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
	_ = role != "" // role is included in UpsertAgentPayload directly
	if role == "" {
		role = db.DefaultAgentRole
	}

	if req.IP == "" {
		req.IP = h.preferredHeartbeatIP(r)
	}

	agent, err := h.deps.DB().GetAgentByNodeID(nodeID)
	if err == nil {
		// Restore soft-deleted agent on reconnection — DEK is preserved
		if agent.DeletedAt != nil {
			if err := h.deps.DB().RestoreDeletedAgent(nodeID); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to restore deleted agent")
				return
			}
			log.Printf("agent: restored deleted agent node=%s (%s)", nodeID, req.Label)
			agent.DeletedAt = nil
		}
		if agent.BlockedAt != nil && agent.BlockReason == "key_version_mismatch" && agent.KeyVersion == req.KeyVersion {
			if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpdateAgentState, clearRebindPayload(nodeID)); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to clear blocked rebind state")
				return
			}
			if agent, err = h.deps.DB().GetAgentByNodeID(nodeID); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to reload agent")
				return
			}
		}
		if agent.BlockedAt != nil {
			respondJSON(w, http.StatusLocked, heartbeatAgentState(agent, "blocked"))
			return
		}
		if agent.RotationRequired {
			if agent.KeyVersion == req.KeyVersion {
				if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpdateAgentState, clearRotationPayload(nodeID)); err != nil {
					respondError(w, http.StatusInternalServerError, "failed to clear rotation state")
					return
				}
				if agent, err = h.deps.DB().GetAgentByNodeID(nodeID); err != nil {
					respondError(w, http.StatusInternalServerError, "failed to reload agent")
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
			if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpdateAgentState, clearRebindPayload(nodeID)); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to clear rebind state")
				return
			}
			if agent, err = h.deps.DB().GetAgentByNodeID(nodeID); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to reload agent")
				return
			}
		}
		if agent.RebindRequired {
			if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpdateAgentState, advanceRebindPayload(nodeID, agent.RebindReason, agent.RetryStage, time.Now().UTC())); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to update rebind state")
				return
			}
			if agent, err = h.deps.DB().GetAgentByNodeID(nodeID); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to reload agent")
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
			if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpdateAgentState, advanceRebindPayload(nodeID, "key_version_mismatch", agent.RetryStage, time.Now().UTC())); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to update rebind state")
				return
			}
			if agent, err = h.deps.DB().GetAgentByNodeID(nodeID); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to reload agent")
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

	// New agent registration requires a valid registration token — consume atomically to prevent race.
	// Exception: trusted IPs can register without a token (for local/dev setups).
	if agent == nil {
		trusted := h.deps.IsTrustedIPString(httputil.ActorIDForRequest(r))
		if req.RegistrationToken != "" {
			// Consume atomically: WHERE status='active' AND expires_at > now
			if err := h.deps.DB().ConsumeRegistrationToken(req.RegistrationToken, nodeID); err != nil {
				respondError(w, http.StatusForbidden, "invalid, expired, or already used registration token")
				return
			}
		} else if !trusted {
			respondError(w, http.StatusForbidden, "registration_token is required for first-time agent registration")
			return
		}
	}

	// Submit all agent state as a single TX — role, capabilities, managed paths
	// included in UpsertAgentPayload to avoid read-after-write race with SubmitTxAsync.
	hostEnabled := agent != nil && agent.HostEnabled
	localEnabled := agent == nil || agent.LocalEnabled
	if req.HostEnabled != nil {
		hostEnabled = *req.HostEnabled
	}
	if req.LocalEnabled != nil {
		localEnabled = *req.LocalEnabled
	}

	upsertPayload := chain.UpsertAgentPayload{
		NodeID:       nodeID,
		Label:        req.Label,
		AgentRole:    role,
		VaultHash:    req.VaultHash,
		VaultName:    req.VaultName,
		HostEnabled:  hostEnabled,
		LocalEnabled: localEnabled,
		ManagedPaths: strings.Join(req.ManagedPaths, ","),
		IP:           req.IP,
		Port:         req.Port,
		SecretsCount: req.SecretsCount,
		ConfigsCount: req.ConfigsCount,
		Version:      req.Version,
		KeyVersion:   req.KeyVersion,
	}
	// Both new and existing agents use Commit to avoid read-after-write race.
	if _, err := h.deps.SubmitTx(r.Context(), chain.TxUpsertAgent, upsertPayload); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to upsert agent")
		return
	}

	agent, err = h.deps.DB().GetAgentByNodeID(nodeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	if agent.AgentHash == "" || (len(agent.DEK) == 0 && len(agent.DEKNonce) == 0) {
		agentHash := agent.AgentHash
		if agentHash == "" {
			var err error
			agentHash, err = generateAgentHash()
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to generate agent hash")
				return
			}
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
			respondError(w, http.StatusInternalServerError, "failed to store agent DEK")
			return
		}

		log.Printf("agent: assigned hash=%s to %s (%s)", agentHash, nodeID, req.Label)

		resp := map[string]interface{}{
			"status":        "registered",
			"vault_id":      httputil.FormatVaultID(req.VaultName, req.VaultHash),
			"managed_paths": db.DecodeManagedPaths(agent.ManagedPaths),
			"key_version":   req.KeyVersion,
		}
		setNodeIdentityAliases(resp, nodeID)
		setRuntimeHashAliases(resp, agentHash)

		// Generate agent_secret for new registrations
		agentSecret, secretErr := generateAgentSecret()
		if secretErr == nil {
			secretHashBytes := sha256.Sum256([]byte(agentSecret))
			secretHash := hex.EncodeToString(secretHashBytes[:])
			encSecret, encNonce, encErr := crypto.Encrypt(kek, []byte(agentSecret))
			if encErr != nil {
				log.Printf("agent: failed to encrypt agent_secret for %s: %v", nodeID, encErr)
			} else if err := h.deps.DB().UpdateAgentSecretHash(nodeID, secretHash, encSecret, encNonce); err != nil {
				log.Printf("agent: failed to store agent_secret_hash for %s: %v", nodeID, err)
			} else {
				resp["agent_secret"] = agentSecret
				log.Printf("agent: issued agent_secret for %s (%s)", nodeID, req.Label)
			}
		} else {
			log.Printf("agent: failed to generate agent_secret for %s: %v", nodeID, secretErr)
		}

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

	// Issue agent_secret if not yet assigned
	if agent.AgentSecretHash == "" {
		agentSecret, secretErr := generateAgentSecret()
		if secretErr == nil {
			kek := h.deps.GetKEK()
			secretHashBytes := sha256.Sum256([]byte(agentSecret))
			secretHash := hex.EncodeToString(secretHashBytes[:])
			encSecret, encNonce, encErr := crypto.Encrypt(kek, []byte(agentSecret))
			if encErr != nil {
				log.Printf("agent: failed to encrypt agent_secret for %s: %v", nodeID, encErr)
			} else if err := h.deps.DB().UpdateAgentSecretHash(nodeID, secretHash, encSecret, encNonce); err != nil {
				log.Printf("agent: failed to store agent_secret_hash for %s: %v", nodeID, err)
			} else {
				resp["agent_secret"] = agentSecret
				log.Printf("agent: issued agent_secret for existing agent %s (%s)", nodeID, agent.Label)
			}
		}
	}

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
