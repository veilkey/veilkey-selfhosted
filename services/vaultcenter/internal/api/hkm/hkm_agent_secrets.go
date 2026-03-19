package hkm

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

func (h *Handler) handleAgentSecrets(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	resp, err := h.deps.HTTPClient().Get(agent.URL() + agentPathSecrets)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}

	var data map[string]interface{}
	if json.Unmarshal(body, &data) == nil {
		if data == nil {
			data = map[string]interface{}{
				"secrets": []interface{}{},
				"count":   0,
			}
		}
		if _, ok := data["secrets"]; !ok || data["secrets"] == nil {
			data["secrets"] = []interface{}{}
		}
		if _, ok := data["count"]; !ok || data["count"] == nil {
			if secrets, ok := data["secrets"].([]interface{}); ok {
				data["count"] = len(secrets)
			} else {
				data["count"] = 0
			}
		}
		if secrets, ok := data["secrets"].([]interface{}); ok {
			for _, item := range secrets {
				if sec, ok := item.(map[string]interface{}); ok {
					if ref, ok := sec["ref"].(string); ok && ref != "" {
						scopeStr, _ := sec["scope"].(string)
						statusStr, _ := sec["status"].(string)
						canonicalRef, fallbackScope, fallbackStatus := normalizeFallbackSecretRef(ref)
						if strings.TrimSpace(scopeStr) == "" && strings.TrimSpace(statusStr) == "" {
							scopeStr, statusStr = fallbackScope, fallbackStatus
						}
						normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVK, refScope(scopeStr), refStatus(statusStr), refScopeTemp)
						if normalizeErr != nil {
							respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+normalizeErr.Error())
							return
						}
						sec["ref"] = canonicalRef
						sec["token"] = makeRef(refFamilyVK, normScope, canonicalRef)
						sec["scope"] = string(normScope)
						sec["status"] = string(normStatus)
						_ = h.upsertTrackedRef(makeRef(refFamilyVK, normScope, canonicalRef), agent.KeyVersion, normStatus, agent.AgentHash)
					}
				}
			}
		}
		data["vault"] = agent.Label
		setRuntimeHashAliases(data, agent.AgentHash)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
