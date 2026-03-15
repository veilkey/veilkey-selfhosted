package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) handleAgentSaveConfigsBulk(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	body, _ := io.ReadAll(r.Body)
	var reqData struct {
		Configs map[string]string `json:"configs"`
		Scope   string            `json:"scope"`
		Status  string            `json:"status"`
	}
	if json.Unmarshal(body, &reqData) != nil || len(reqData.Configs) == 0 {
		s.respondError(w, http.StatusBadRequest, "configs map is required")
		return
	}
	for key := range reqData.Configs {
		if !isValidResourceName(key) {
			s.respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
			return
		}
	}
	scope, status, err := normalizeScopeStatus("VE", reqData.Scope, reqData.Status, "LOCAL")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), "PUT", agent.URL()+"/api/configs/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		for key := range reqData.Configs {
			_ = s.upsertTrackedRef("VE:"+scope+":"+key, agent.KeyVersion, status, agent.AgentHash)
			s.saveAuditEvent(
				"config",
				"VE:"+scope+":"+key,
				"save",
				"agent",
				agent.AgentHash,
				"bulk_update",
				"agent_save_configs_bulk",
				nil,
				map[string]any{
					"key":                key,
					"ref":                "VE:" + scope + ":" + key,
					"vault_runtime_hash": agent.AgentHash,
					"status":             status,
				},
			)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
