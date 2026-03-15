package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

func (s *Server) handleAgentConfigs(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), "GET", agent.URL()+"/api/configs", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	var data map[string]interface{}
	if json.Unmarshal(body, &data) == nil {
		if configs, ok := data["configs"].([]interface{}); ok {
			for _, item := range configs {
				if cfg, ok := item.(map[string]interface{}); ok {
					if key, ok := cfg["key"].(string); ok && key != "" {
						scope, _ := cfg["scope"].(string)
						status, _ := cfg["status"].(string)
						scope, status, err = normalizeScopeStatus("VE", scope, status, "LOCAL")
						if err != nil {
							s.respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+err.Error())
							return
						}
						cfg["ref"] = "VE:" + scope + ":" + key
						cfg["scope"] = scope
						cfg["status"] = status
						_ = s.upsertTrackedRef("VE:"+scope+":"+key, agent.KeyVersion, status, agent.AgentHash)
					}
				}
			}
		}
		data["vault"] = agent.Label
		setRuntimeHashAliases(data, agent.AgentHash)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
