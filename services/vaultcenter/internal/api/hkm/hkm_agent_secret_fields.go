package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

type agentSecretField struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

func (h *Handler) handleAgentSaveSecretFields(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAgentAccess(r) {
		respondError(w, http.StatusForbidden, "agent access denied")
		return
	}

	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	var req struct {
		Fields []agentSecretField `json:"fields"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil || len(req.Fields) == 0 {
		respondError(w, http.StatusBadRequest, "fields are required")
		return
	}

	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent, name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta == nil || meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope")
		return
	}
	if meta.Status != string(refStatusActive) || (meta.Scope != string(refScopeLocal) && meta.Scope != string(refScopeExternal)) {
		respondError(w, http.StatusConflict, "additional secret fields require VK:LOCAL or VK:EXTERNAL active lifecycle")
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}

	payloadFields := make([]map[string]interface{}, 0, len(req.Fields))
	responseFields := make([]map[string]string, 0, len(req.Fields))
	for _, field := range req.Fields {
		if !isValidResourceName(field.Key) || field.Value == "" {
			respondError(w, http.StatusBadRequest, "field key and value are required")
			return
		}
		ciphertext, nonce, err := crypto.Encrypt(agentDEK, []byte(field.Value))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to encrypt secret field")
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

	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":   name,
		"fields": payloadFields,
	})
	fieldsReq, _ := http.NewRequest(http.MethodPost, agent.URL()+agentPathSecretFields, bytes.NewReader(reqBody))
	fieldsReq.Header.Set("Content-Type", httputil.ContentTypeJSON)
	h.setAgentAuthHeader(fieldsReq, agent)
	resp, err := h.deps.HTTPClient().Do(fieldsReq)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"fields":             responseFields,
		"saved":              len(responseFields),
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (h *Handler) handleAgentGetSecretField(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAgentAccess(r) {
		respondError(w, http.StatusForbidden, "agent access denied")
		return
	}

	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	fieldKey := r.PathValue("field")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent, name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta == nil || meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope")
		return
	}

	cipher, err := h.fetchAgentFieldCiphertext(agent, meta.Ref, fieldKey)
	if err != nil {
		if strings.Contains(err.Error(), "agent returned 404") {
			respondError(w, http.StatusNotFound, "secret field not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch secret field ciphertext")
		return
	}

	agentDEK, err := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to decrypt agent DEK")
		return
	}
	plaintext, err := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":               name,
		"field":              fieldKey,
		"type":               cipher.FieldType,
		"value":              string(plaintext),
		"ref":                meta.Ref,
		"token":              makeRef(refFamilyVK, refScope(meta.Scope), meta.Ref),
		"scope":              meta.Scope,
		"status":             meta.Status,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (h *Handler) handleAgentDeleteSecretField(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAgentAccess(r) {
		respondError(w, http.StatusForbidden, "agent access denied")
		return
	}

	hashOrLabel := r.PathValue("agent")
	name := r.PathValue("name")
	fieldKey := r.PathValue("field")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, joinPath(agent.URL(), agentPathSecrets, name, "fields", fieldKey), nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to build delete request")
		return
	}
	h.setAgentAuthHeader(deleteReq, agent)
	resp, err := h.deps.HTTPClient().Do(deleteReq)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", httputil.ContentTypeJSON)
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
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
