package hkm

import (
	"io"
	"net/http"
	"strings"
)

func (h *Handler) handleAgentDeleteSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")

	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	var trackedRef string
	meta, status, _, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err == nil && status == http.StatusOK && meta != nil && meta.Ref != "" {
		if err := normalizeMeta(meta); err == nil {
			trackedRef = meta.Token
		}
	}

	req, _ := http.NewRequest(http.MethodDelete, agent.URL()+"/api/secrets/"+name, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK && trackedRef != "" {
		_ = h.deleteTrackedRef(trackedRef)
		if metaRefParts := strings.Split(trackedRef, ":"); len(metaRefParts) == 3 {
			_ = h.deleteTrackedRef("VK:TEMP:" + metaRefParts[2])
		}
		h.deps.SaveAuditEvent(
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
