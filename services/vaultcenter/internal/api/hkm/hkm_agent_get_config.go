package hkm

import (
	"encoding/json"
	"io"
	"net/http"

	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentGetConfig(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	key := r.PathValue("key")

	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, joinPath(agent.URL(), agentPathConfigs, key), nil)
	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	if resp.StatusCode == http.StatusOK {
		var data map[string]interface{}
		if json.Unmarshal(body, &data) == nil {
			scopeStr, _ := data["scope"].(string)
			statusStr, _ := data["status"].(string)
			normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVE, refScope(scopeStr), refStatus(statusStr), refScopeLocal)
			if normalizeErr != nil {
				respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+normalizeErr.Error())
				return
			}
			data["ref"] = makeRef(refFamilyVE, normScope, key)
			data["scope"] = string(normScope)
			data["status"] = string(normStatus)
			data["vault"] = agent.Label
			setRuntimeHashAliases(data, agent.AgentHash)
			_ = h.upsertTrackedRef(r.Context(), makeRef(refFamilyVE, normScope, key), agent.KeyVersion, normStatus, agent.AgentHash)
			if marshaled, marshalErr := json.Marshal(data); marshalErr == nil {
				body = marshaled
			}
		}
	}
	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
