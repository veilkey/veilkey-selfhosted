package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func (h *Handler) handleAgentSaveConfigsBulk(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var reqData struct {
		Configs map[string]string `json:"configs"`
		Scope   string            `json:"scope"`
		Status  string            `json:"status"`
	}
	if json.Unmarshal(body, &reqData) != nil || len(reqData.Configs) == 0 {
		respondError(w, http.StatusBadRequest, "configs map is required")
		return
	}
	for key := range reqData.Configs {
		if !isValidResourceName(key) {
			respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
			return
		}
	}
	scope, status, err := normalizeScopeStatus(refFamilyVE, reqData.Scope, reqData.Status, refScopeLocal)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPut, agent.URL()+agentPathConfigsBulk, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	if resp.StatusCode == http.StatusOK {
		for key := range reqData.Configs {
			_ = h.upsertTrackedRef(makeRef(refFamilyVE, scope, key), agent.KeyVersion, status, agent.AgentHash)
			h.deps.SaveAuditEvent(
				"config",
				makeRef(refFamilyVE, scope, key),
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
