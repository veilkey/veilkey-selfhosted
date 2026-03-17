package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) handleAgentSaveConfig(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var reqData struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &reqData); err != nil || reqData.Key == "" || reqData.Value == "" {
		s.respondError(w, http.StatusBadRequest, "key and value are required")
		return
	}
	if !isValidResourceName(reqData.Key) {
		s.respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}
	scope, _, err := normalizeScopeStatus("VE", reqData.Scope, reqData.Status, "LOCAL")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), "POST", agent.URL()+"/api/configs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	if resp.StatusCode == http.StatusOK {
		var respData map[string]interface{}
		if json.Unmarshal(respBody, &respData) == nil {
			key := reqData.Key
			respScope, _ := respData["scope"].(string)
			respStatus, _ := respData["status"].(string)
			scope, status, err = normalizeScopeStatus("VE", respScope, respStatus, scope)
			if err != nil {
				s.respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+err.Error())
				return
			}
			respData["ref"] = "VE:" + scope + ":" + key
			respData["scope"] = scope
			respData["status"] = status
			respData["vault"] = agent.Label
			setRuntimeHashAliases(respData, agent.AgentHash)
			_ = s.upsertTrackedRef("VE:"+scope+":"+key, agent.KeyVersion, status, agent.AgentHash)
			s.saveAuditEvent(
				"config",
				"VE:"+scope+":"+key,
				"save",
				"agent",
				agent.AgentHash,
				"",
				"agent_save_config",
				nil,
				map[string]any{
					"key":                key,
					"ref":                "VE:" + scope + ":" + key,
					"vault_runtime_hash": agent.AgentHash,
					"status":             status,
				},
			)
			if marshaled, marshalErr := json.Marshal(respData); marshalErr == nil {
				respBody = marshaled
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
