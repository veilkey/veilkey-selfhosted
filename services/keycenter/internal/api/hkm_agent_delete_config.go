package api

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) handleAgentDeleteConfig(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	key := r.PathValue("key")

	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	trackedRef := ""
	preReq, _ := http.NewRequestWithContext(r.Context(), "GET", agent.URL()+"/api/configs/"+key, nil)
	preResp, err := http.DefaultClient.Do(preReq)
	if err == nil {
		defer preResp.Body.Close()
		if preResp.StatusCode == http.StatusOK {
			var data struct {
				Scope string `json:"scope"`
			}
			body, readErr := io.ReadAll(preResp.Body)
			if readErr == nil && json.Unmarshal(body, &data) == nil {
				scope, _, normalizeErr := normalizeScopeStatus("VE", data.Scope, "", "LOCAL")
				if normalizeErr == nil {
					trackedRef = "VE:" + scope + ":" + key
				}
			}
		}
	}

	req, _ := http.NewRequestWithContext(r.Context(), "DELETE", agent.URL()+"/api/configs/"+key, nil)
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
	if resp.StatusCode == http.StatusOK && trackedRef != "" {
		_ = s.deleteTrackedRef(trackedRef)
		s.saveAuditEvent(
			"config",
			trackedRef,
			"delete",
			"agent",
			agent.AgentHash,
			"",
			"agent_delete_config",
			map[string]any{
				"key": key,
				"ref": trackedRef,
			},
			map[string]any{
				"key": key,
			},
		)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
