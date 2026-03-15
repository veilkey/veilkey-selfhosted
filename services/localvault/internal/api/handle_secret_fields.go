package api

import (
	"encoding/json"
	"net/http"

	"veilkey-localvault/internal/db"
)

type secretFieldMeta struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

func normalizeSecretFieldType(raw string) string {
	switch raw {
	case "login", "otp", "password", "key", "url":
		return raw
	default:
		return "text"
	}
}

func (s *Server) handleSaveSecretFields(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Fields []struct {
			Key        string `json:"key"`
			Type       string `json:"type"`
			Ciphertext []byte `json:"ciphertext"`
			Nonce      []byte `json:"nonce"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Fields) == 0 {
		s.respondError(w, http.StatusBadRequest, "name and fields are required")
		return
	}
	if len(req.Fields) > maxBulkItems {
		s.respondError(w, http.StatusBadRequest, "too many fields (max 200)")
		return
	}

	secret, err := s.db.GetSecretByName(req.Name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if secret.Status != "active" || (secret.Scope != "LOCAL" && secret.Scope != "EXTERNAL") {
		s.respondError(w, http.StatusConflict, "secret fields require VK:LOCAL or VK:EXTERNAL active lifecycle")
		return
	}

	fields := make([]db.SecretField, 0, len(req.Fields))
	meta := make([]secretFieldMeta, 0, len(req.Fields))
	for _, field := range req.Fields {
		if !isValidResourceName(field.Key) {
			s.respondError(w, http.StatusBadRequest, "field key must match [A-Z_][A-Z0-9_]*")
			return
		}
		if len(field.Ciphertext) == 0 || len(field.Nonce) == 0 {
			s.respondError(w, http.StatusBadRequest, "field ciphertext and nonce are required")
			return
		}
		fieldType := normalizeSecretFieldType(field.Type)
		fields = append(fields, db.SecretField{
			SecretName: req.Name,
			FieldKey:   field.Key,
			FieldType:  fieldType,
			Ciphertext: field.Ciphertext,
			Nonce:      field.Nonce,
		})
		meta = append(meta, secretFieldMeta{Key: field.Key, Type: fieldType})
	}

	if err := s.db.SaveSecretFields(req.Name, fields); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save secret fields: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":   req.Name,
		"fields": meta,
		"saved":  len(meta),
	})
}

func (s *Server) handleCipherField(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	fieldKey := r.PathValue("field")
	if ref == "" || fieldKey == "" {
		s.respondError(w, http.StatusBadRequest, "ref and field are required")
		return
	}
	if !isValidResourceName(fieldKey) {
		s.respondError(w, http.StatusBadRequest, "field key must match [A-Z_][A-Z0-9_]*")
		return
	}

	secret, err := s.db.GetSecretByRef(ref)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "ref not found: "+ref)
		return
	}
	if secret.Status == "block" {
		s.respondError(w, http.StatusLocked, "ref is blocked: VK:"+secret.Scope+":"+ref)
		return
	}

	field, err := s.db.GetSecretField(secret.Name, fieldKey)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":       secret.Name,
		"field":      field.FieldKey,
		"type":       field.FieldType,
		"ciphertext": field.Ciphertext,
		"nonce":      field.Nonce,
		"version":    secret.Version,
		"ref":        ref,
	})
}

func (s *Server) handleDeleteSecretField(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fieldKey := r.PathValue("field")
	if name == "" || fieldKey == "" {
		s.respondError(w, http.StatusBadRequest, "name and field are required")
		return
	}
	if !isValidResourceName(name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}
	if !isValidResourceName(fieldKey) {
		s.respondError(w, http.StatusBadRequest, "field key must match [A-Z_][A-Z0-9_]*")
		return
	}

	secret, err := s.db.GetSecretByName(name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if secret.Status != "active" || (secret.Scope != "LOCAL" && secret.Scope != "EXTERNAL") {
		s.respondError(w, http.StatusConflict, "secret fields require VK:LOCAL or VK:EXTERNAL active lifecycle")
		return
	}
	if err := s.db.DeleteSecretField(name, fieldKey); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"name":    name,
		"deleted": fieldKey,
	})
}
