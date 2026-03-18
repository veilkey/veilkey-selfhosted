package hkm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func (h *Handler) handleAgentMigrate(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	if len(agent.DEK) == 0 {
		respondError(w, http.StatusBadRequest, "agent has no Hub-managed DEK")
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	rekeyBody, err := json.Marshal(map[string]interface{}{
		"dek":     agentDEK,
		"version": 100,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to marshal rekey request")
		return
	}

	resp, err := h.deps.HTTPClient().Post(agent.URL()+"/api/rekey", "application/json", bytes.NewReader(rekeyBody))
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to read agent response body")
		return
	}
	var rekeyResult map[string]interface{}
	_ = json.Unmarshal(body, &rekeyResult)

	if resp.StatusCode != http.StatusOK {
		respondError(w, resp.StatusCode, fmt.Sprintf("rekey failed: %s", string(body)))
		return
	}

	log.Printf("agent: migrated %s (%s) to Hub-managed DEK", agent.Label, agent.AgentHash)

	payload := map[string]interface{}{
		"status": "migrated",
		"vault":  agent.Label,
		"rekey":  rekeyResult,
	}
	setRuntimeHashAliases(payload, agent.AgentHash)
	respondJSON(w, http.StatusOK, payload)
}
