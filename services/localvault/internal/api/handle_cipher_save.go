package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"veilkey-localvault/internal/crypto"
	"veilkey-localvault/internal/db"
)

func (s *Server) handleSaveCipher(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Ref        string `json:"ref"`
		Ciphertext []byte `json:"ciphertext"`
		Nonce      []byte `json:"nonce"`
		Version    int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Ciphertext) == 0 || len(req.Nonce) == 0 {
		s.respondError(w, http.StatusBadRequest, "name, ciphertext, and nonce are required")
		return
	}

	if !isValidResourceName(req.Name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	nodeInfo, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	if req.Version == 0 {
		req.Version = nodeInfo.Version
	}

	existing, _ := s.db.GetSecretByName(req.Name)
	id := crypto.GenerateUUID()
	ref := req.Ref
	action := "created"
	scope := "TEMP"
	status := "temp"
	if existing != nil {
		id = existing.ID
		ref = existing.Ref
		action = "updated"
		if existing.Scope != "" {
			scope = existing.Scope
		}
		if existing.Status != "" {
			status = existing.Status
		}
	}
	if ref == "" {
		ref, err = generateSecretRef(8)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to generate ref")
			return
		}
	}

	secret := &db.Secret{
		ID:         id,
		Name:       req.Name,
		Ref:        ref,
		Ciphertext: req.Ciphertext,
		Nonce:      req.Nonce,
		Version:    req.Version,
		Scope:      scope,
		Status:     status,
	}
	if err := s.db.SaveSecret(secret); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save secret: "+err.Error())
		return
	}

	parsed := ParsedRef{Family: RefFamilyVK, Scope: RefScope(scope), ID: ref}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"name":    req.Name,
		"ref":     ref,
		"token":   parsed.CanonicalString(),
		"version": req.Version,
		"scope":   scope,
		"status":  status,
		"action":  action,
	})
}

func (s *Server) handleGetSecretMeta(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "secret name is required")
		return
	}
	if !isValidResourceName(name) {
		s.respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	secret, err := s.db.GetSecretByName(name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	resp := map[string]interface{}{
		"name":             secret.Name,
		"display_name":     secret.DisplayName,
		"description":      secret.Description,
		"tags_json":        secret.TagsJSON,
		"origin":           secret.Origin,
		"class":            secret.Class,
		"version":          secret.Version,
		"status":           secret.Status,
		"last_rotated_at":  nullableNullTime(secret.LastRotatedAt),
		"last_revealed_at": nullableNullTime(secret.LastRevealedAt),
	}
	fields, err := s.db.ListSecretFields(secret.Name)
	if err == nil && len(fields) > 0 {
		meta := make([]map[string]interface{}, 0, len(fields))
		for _, field := range fields {
			meta = append(meta, map[string]interface{}{
				"key":               field.FieldKey,
				"type":              field.FieldType,
				"field_role":        field.FieldRole,
				"display_name":      field.DisplayName,
				"masked_by_default": field.MaskedByDefault,
				"required":          field.Required,
				"sort_order":        field.SortOrder,
			})
		}
		resp["fields"] = meta
		resp["fields_count"] = len(meta)
	}
	if secret.Ref != "" {
		resp["ref"] = secret.Ref
		resp["token"] = ParsedRef{Family: RefFamilyVK, Scope: RefScope(secret.Scope), ID: secret.Ref}.CanonicalString()
		resp["scope"] = secret.Scope
	}
	s.respondJSON(w, http.StatusOK, resp)
}

func nullableNullTime(value sql.NullTime) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339)
}
