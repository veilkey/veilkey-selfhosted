package hkm

import (
	"encoding/json"
	"io"
	"net/http"

	"veilkey-vaultcenter/internal/httputil"
)

func (h *Handler) handleAgentDeleteConfig(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	key := r.PathValue("key")

	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	trackedRef := ""
	preReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, joinPath(agent.URL(), agentPathConfigs, key), nil)
	h.setAgentAuthHeader(preReq, agent)
	preResp, err := h.deps.HTTPClient().Do(preReq)
	if err == nil {
		defer preResp.Body.Close()
		if preResp.StatusCode == http.StatusOK {
			var data struct {
				Scope string `json:"scope"`
			}
			body, readErr := io.ReadAll(preResp.Body)
			if readErr == nil && json.Unmarshal(body, &data) == nil {
				normScope, _, normalizeErr := normalizeScopeStatus(refFamilyVE, refScope(data.Scope), "", refScopeLocal)
				if normalizeErr == nil {
					trackedRef = makeRef(refFamilyVE, normScope, key)
				}
			}
		}
	}

	req, _ := http.NewRequestWithContext(r.Context(), http.MethodDelete, joinPath(agent.URL(), agentPathConfigs, key), nil)
	h.setAgentAuthHeader(req, agent)
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
	if resp.StatusCode == http.StatusOK && trackedRef != "" {
		_ = h.deleteTrackedRef(r.Context(), trackedRef)
	}
	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
