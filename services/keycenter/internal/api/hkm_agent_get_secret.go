package api

import (
	"encoding/json"
	"io"
	"net/http"
	"veilkey-keycenter/internal/crypto"
)

func (s *Server) handleAgentGetSecret(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")

	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	agentURL := agent.URL()
	resp, err := s.httpClient.Get(agentURL + "/api/secrets/meta/" + name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	var secretData struct {
		Name   string `json:"name"`
		Ref    string `json:"ref"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
		Fields []struct {
			Key  string `json:"key"`
			Type string `json:"type"`
		} `json:"fields"`
		FieldsCount int `json:"fields_count"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &secretData); err != nil {
		s.respondError(w, http.StatusInternalServerError, "invalid agent response")
		return
	}

	if secretData.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	secretData.Scope, secretData.Status, err = normalizeScopeStatus("VK", secretData.Scope, secretData.Status, "TEMP")
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef("VK:"+secretData.Scope+":"+secretData.Ref, agent.KeyVersion, secretData.Status, agent.AgentHash)

	cipher, err := s.fetchAgentCiphertext(agentURL, secretData.Ref)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to fetch ciphertext: "+err.Error())
		return
	}

	plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	payload := map[string]interface{}{
		"name":         name,
		"value":        string(plaintext),
		"ref":          secretData.Ref,
		"token":        "VK:" + secretData.Scope + ":" + secretData.Ref,
		"scope":        secretData.Scope,
		"status":       secretData.Status,
		"vault":        agent.Label,
		"fields":       secretData.Fields,
		"fields_count": secretData.FieldsCount,
	}
	setRuntimeHashAliases(payload, agent.AgentHash)
	s.respondJSON(w, http.StatusOK, payload)
}
