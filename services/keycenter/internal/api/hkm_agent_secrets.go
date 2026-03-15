package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

func (s *Server) handleAgentSecrets(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	resp, err := http.Get(agent.URL() + "/api/secrets")
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
		if secrets, ok := data["secrets"].([]interface{}); ok {
			for _, item := range secrets {
				if sec, ok := item.(map[string]interface{}); ok {
					if ref, ok := sec["ref"].(string); ok && ref != "" {
						scope, _ := sec["scope"].(string)
						status, _ := sec["status"].(string)
						scope, status, err = normalizeScopeStatus("VK", scope, status, "TEMP")
						if err != nil {
							s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
							return
						}
						sec["token"] = "VK:" + scope + ":" + ref
						sec["scope"] = scope
						sec["status"] = status
						_ = s.upsertTrackedRef("VK:"+scope+":"+ref, agent.KeyVersion, status, agent.AgentHash)
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
