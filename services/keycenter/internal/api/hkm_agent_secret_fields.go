package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"veilkey-keycenter/internal/crypto"
)

type agentSecretField struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

func (s *Server) handleAgentSaveSecretFields(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	var req struct {
		Fields []agentSecretField `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Fields) == 0 {
		s.respondError(w, http.StatusBadRequest, "fields are required")
		return
	}

	metaResp, err := http.Get(agent.URL() + "/api/secrets/meta/" + name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer metaResp.Body.Close()
	if metaResp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(metaResp.Body)
		if readErr != nil {
			body = []byte(`{"error":"(unreadable body)"}`)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(metaResp.StatusCode)
		w.Write(body)
		return
	}

	var meta struct {
		Name   string `json:"name"`
		Ref    string `json:"ref"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(metaResp.Body).Decode(&meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "invalid agent metadata response")
		return
	}
	if meta.Status != "active" || (meta.Scope != "LOCAL" && meta.Scope != "EXTERNAL") {
		s.respondError(w, http.StatusConflict, "additional secret fields require VK:LOCAL or VK:EXTERNAL active lifecycle")
		return
	}

	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	payloadFields := make([]map[string]interface{}, 0, len(req.Fields))
	responseFields := make([]map[string]string, 0, len(req.Fields))
	for _, field := range req.Fields {
		if !isValidResourceName(field.Key) || field.Value == "" {
			s.respondError(w, http.StatusBadRequest, "field key and value are required")
			return
		}
		ciphertext, nonce, err := crypto.Encrypt(agentDEK, []byte(field.Value))
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to encrypt secret field")
			return
		}
		fieldType := normalizeFieldType(field.Type)
		payloadFields = append(payloadFields, map[string]interface{}{
			"key":        field.Key,
			"type":       fieldType,
			"ciphertext": ciphertext,
			"nonce":      nonce,
		})
		responseFields = append(responseFields, map[string]string{"key": field.Key, "type": fieldType})
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":   name,
		"fields": payloadFields,
	})
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to marshal request body")
		return
	}
	resp, err := http.Post(agent.URL()+"/api/secrets/fields", "application/json", bytes.NewReader(body))
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
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"name":               name,
		"ref":                meta.Ref,
		"token":              "VK:" + meta.Scope + ":" + meta.Ref,
		"scope":              meta.Scope,
		"fields":             responseFields,
		"saved":              len(responseFields),
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	}); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

func (s *Server) handleAgentGetSecretField(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	fieldKey := r.PathValue("field")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	metaResp, err := http.Get(agent.URL() + "/api/secrets/meta/" + name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	defer metaResp.Body.Close()
	if metaResp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(metaResp.Body)
		if readErr != nil {
			body = []byte(`{"error":"(unreadable body)"}`)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(metaResp.StatusCode)
		w.Write(body)
		return
	}

	var secretData struct {
		Ref    string `json:"ref"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(metaResp.Body).Decode(&secretData); err != nil {
		s.respondError(w, http.StatusBadGateway, "invalid agent response")
		return
	}
	if secretData.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}

	cipher, err := s.fetchAgentFieldCiphertext(agent.URL(), secretData.Ref, fieldKey)
	if err != nil {
		if strings.Contains(err.Error(), "agent returned 404") {
			s.respondError(w, http.StatusNotFound, "secret field not found")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "failed to fetch secret field ciphertext: "+err.Error())
		return
	}

	agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}
	plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":               name,
		"field":              fieldKey,
		"type":               cipher.FieldType,
		"value":              string(plaintext),
		"ref":                secretData.Ref,
		"token":              fmt.Sprintf("VK:%s:%s", secretData.Scope, secretData.Ref),
		"scope":              secretData.Scope,
		"status":             secretData.Status,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (s *Server) handleAgentDeleteSecretField(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	fieldKey := r.PathValue("field")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	req, err := http.NewRequest(http.MethodDelete, agent.URL()+"/api/secrets/"+name+"/fields/"+fieldKey, nil)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to build delete request")
		return
	}
	resp, err := http.DefaultClient.Do(req)
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
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func normalizeFieldType(raw string) string {
	switch raw {
	case "login", "otp", "password", "key", "url":
		return raw
	default:
		return "text"
	}
}
