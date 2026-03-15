package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func (s *Server) handleAgentMigrate(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	if len(agent.DEK) == 0 {
		s.respondError(w, http.StatusBadRequest, "agent has no Hub-managed DEK")
		return
	}

	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	rekeyBody, err := json.Marshal(map[string]interface{}{
		"dek":     agentDEK,
		"version": 100,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to marshal rekey request")
		return
	}

	resp, err := http.Post(agent.URL()+"/api/rekey", "application/json", bytes.NewReader(rekeyBody))
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
	var rekeyResult map[string]interface{}
	_ = json.Unmarshal(body, &rekeyResult)

	if resp.StatusCode != http.StatusOK {
		s.respondError(w, resp.StatusCode, fmt.Sprintf("rekey failed: %s", string(body)))
		return
	}

	log.Printf("agent: migrated %s (%s) to Hub-managed DEK", agent.Label, agent.AgentHash)

	payload := map[string]interface{}{
		"status": "migrated",
		"vault":  agent.Label,
		"rekey":  rekeyResult,
	}
	setRuntimeHashAliases(payload, agent.AgentHash)
	s.respondJSON(w, http.StatusOK, payload)
}
