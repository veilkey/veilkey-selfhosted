package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) handleAgentDeleteSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")

	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	var trackedRef string
	metaResp, err := s.httpClient.Get(agent.URL() + "/api/secrets/meta/" + name)
	if err == nil {
		defer metaResp.Body.Close()
		if metaResp.StatusCode == http.StatusOK {
			var meta struct {
				Ref   string `json:"ref"`
				Scope string `json:"scope"`
			}
			body, readErr := io.ReadAll(metaResp.Body)
			if readErr == nil && json.Unmarshal(body, &meta) == nil && meta.Ref != "" {
				if meta.Scope == "" {
					meta.Scope = "LOCAL"
				}
				trackedRef = "VK:" + meta.Scope + ":" + meta.Ref
			}
		}
	}

	req, _ := http.NewRequest("DELETE", agent.URL()+"/api/secrets/"+name, nil)
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
	if resp.StatusCode == http.StatusOK && trackedRef != "" {
		_ = s.deleteTrackedRef(trackedRef)
		if metaRefParts := strings.Split(trackedRef, ":"); len(metaRefParts) == 3 {
			_ = s.deleteTrackedRef("VK:TEMP:" + metaRefParts[2])
		}
		s.saveAuditEvent(
			"secret",
			trackedRef,
			"delete",
			"agent",
			agent.AgentHash,
			"",
			"agent_delete_secret",
			map[string]any{
				"name": trackedRef,
				"ref":  trackedRef,
			},
			map[string]any{
				"name": name,
			},
		)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
