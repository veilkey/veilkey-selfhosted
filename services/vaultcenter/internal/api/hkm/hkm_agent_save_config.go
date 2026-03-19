package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentSaveConfig(w http.ResponseWriter, r *http.Request) {
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
		Key    string `json:"key"`
		Value  string `json:"value"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &reqData); err != nil || reqData.Key == "" || reqData.Value == "" {
		respondError(w, http.StatusBadRequest, "key and value are required")
		return
	}
	if !isValidResourceName(reqData.Key) {
		respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}
	normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVE, refScope(reqData.Scope), refStatus(reqData.Status), refScopeLocal)
	if normalizeErr != nil {
		respondError(w, http.StatusBadRequest, normalizeErr.Error())
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, agent.URL()+agentPathConfigs, bytes.NewReader(body))
	req.Header.Set("Content-Type", httputil.ContentTypeJSON)
	resp, err := h.deps.HTTPClient().Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	if resp.StatusCode == http.StatusOK {
		var respData map[string]interface{}
		if json.Unmarshal(respBody, &respData) == nil {
			key := reqData.Key
			respScopeStr, _ := respData["scope"].(string)
			respStatusStr, _ := respData["status"].(string)
			normScope, normStatus, normalizeErr = normalizeScopeStatus(refFamilyVE, refScope(respScopeStr), refStatus(respStatusStr), normScope)
			if normalizeErr != nil {
				respondError(w, http.StatusBadGateway, "agent returned unsupported config scope: "+normalizeErr.Error())
				return
			}
			respData["ref"] = makeRef(refFamilyVE, normScope, key)
			respData["scope"] = string(normScope)
			respData["status"] = string(normStatus)
			respData["vault"] = agent.Label
			setRuntimeHashAliases(respData, agent.AgentHash)
			_ = h.upsertTrackedRef(r.Context(), makeRef(refFamilyVE, normScope, key), agent.KeyVersion, normStatus, agent.AgentHash)
			if marshaled, marshalErr := json.Marshal(respData); marshalErr == nil {
				respBody = marshaled
			}
		}
	}
	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
