package api

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) handleAgentGetConfig(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	key := r.PathValue("key")

	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	req, _ := http.NewRequestWithContext(r.Context(), "GET", agent.URL()+"/api/configs/"+key, nil)
	resp, err := s.httpClient.Do(req)
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
	if resp.StatusCode == http.StatusOK {
		var data map[string]interface{}
		if json.Unmarshal(body, &data) == nil {
			scope, _ := data["scope"].(string)
			status, _ := data["status"].(string)
			scope, status, err = normalizeScopeStatus("VE", scope, status, "LOCAL")
			if err != nil {
				s.respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+err.Error())
				return
			}
			data["ref"] = "VE:" + scope + ":" + key
			data["scope"] = scope
			data["status"] = status
			data["vault"] = agent.Label
			setRuntimeHashAliases(data, agent.AgentHash)
			_ = s.upsertTrackedRef("VE:"+scope+":"+key, agent.KeyVersion, status, agent.AgentHash)
			if marshaled, marshalErr := json.Marshal(data); marshalErr == nil {
				body = marshaled
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
