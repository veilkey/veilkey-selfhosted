package hkm

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentConfigs(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, agent.URL()+agentPathConfigs, nil)
	resp, err := h.deps.HTTPClient().Do(req)
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
		if configs, ok := data["configs"].([]interface{}); ok {
			for _, item := range configs {
				if cfg, ok := item.(map[string]interface{}); ok {
					if key, ok := cfg["key"].(string); ok && key != "" {
						scopeStr, _ := cfg["scope"].(string)
						statusStr, _ := cfg["status"].(string)
						normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVE, refScope(scopeStr), refStatus(statusStr), refScopeLocal)
						if normalizeErr != nil {
							respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+normalizeErr.Error())
							return
						}
						cfg["ref"] = makeRef(refFamilyVE, normScope, key)
						cfg["scope"] = string(normScope)
						cfg["status"] = string(normStatus)
						_ = h.upsertTrackedRef(r.Context(), makeRef(refFamilyVE, normScope, key), agent.KeyVersion, normStatus, agent.AgentHash)
					}
				}
			}
		}
		data["vault"] = agent.Label
		setRuntimeHashAliases(data, agent.AgentHash)
	}

	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
