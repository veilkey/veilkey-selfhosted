package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"veilkey-keycenter/internal/crypto"
)

func (s *Server) handleAgentSaveSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Value == "" {
		s.respondError(w, http.StatusBadRequest, "name and value are required")
		return
	}
	if !isValidResourceName(req.Name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	ciphertext, nonce, err := crypto.Encrypt(agentDEK, []byte(req.Value))
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to encrypt agent secret")
		return
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":       req.Name,
		"ciphertext": ciphertext,
		"nonce":      nonce,
		"version":    0,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}
	resp, err := http.Post(agent.URL()+"/api/cipher", "application/json", bytes.NewReader(body))
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

	var data map[string]interface{}
	if json.Unmarshal(respBody, &data) == nil {
		if ref, ok := data["ref"].(string); ok && ref != "" {
			scope, _ := data["scope"].(string)
			status, _ := data["status"].(string)
			scope, status, err = normalizeScopeStatus("VK", scope, status, "TEMP")
			if err != nil {
				s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
				return
			}
			canonical := "VK:" + scope + ":" + ref
			data["token"] = canonical
			data["scope"] = scope
			data["status"] = status
			_ = s.upsertTrackedRefNamed(canonical, agent.KeyVersion, status, agent.AgentHash, req.Name)
			s.saveAuditEvent(
				"secret",
				canonical,
				"save",
				"agent",
				agent.AgentHash,
				"",
				"agent_save_secret",
				nil,
				map[string]any{
					"name":               req.Name,
					"ref":                canonical,
					"vault_runtime_hash": agent.AgentHash,
					"status":             status,
				},
			)
		}
		data["vault"] = agent.Label
		setRuntimeHashAliases(data, agent.AgentHash)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
