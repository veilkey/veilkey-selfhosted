package secrets

import (
	"encoding/json"
	"net/http"
	"time"

	"veilkey-localvault/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func (h *Handler) handleSaveCipher(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Ref        string `json:"ref"`
		Ciphertext []byte `json:"ciphertext"`
		Nonce      []byte `json:"nonce"`
		Version    int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || len(req.Ciphertext) == 0 || len(req.Nonce) == 0 {
		respondError(w, http.StatusBadRequest, "name, ciphertext, and nonce are required")
		return
	}
	if !isValidResourceName(req.Name) {
		respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	nodeInfo, err := h.deps.DB().GetNodeInfo()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	if req.Version == 0 {
		req.Version = nodeInfo.Version
	}

	existing, _ := h.deps.DB().GetSecretByName(req.Name)
	id := crypto.GenerateUUID()
	ref := req.Ref
	action := "created"
	scope := refScopeTemp
	status := refStatusTemp
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
		ref, err = crypto.GenerateHexRef(8)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate ref")
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
	if err := h.deps.DB().SaveSecret(secret); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save secret: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"name":    req.Name,
		"ref":     ref,
		"token":   vkRef(scope, ref),
		"version": req.Version,
		"scope":   scope,
		"status":  status,
		"action":  action,
	})
}

func (h *Handler) handleGetSecretMeta(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "secret name is required")
		return
	}
	if !isValidResourceName(name) {
		respondError(w, http.StatusBadRequest, "name must match [A-Z_][A-Z0-9_]*")
		return
	}

	secret, err := h.deps.DB().GetSecretByName(name)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
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
	if fields, err := h.deps.DB().ListSecretFields(secret.Name); err == nil && len(fields) > 0 {
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
		resp["token"] = vkRef(secret.Scope, secret.Ref)
		resp["scope"] = secret.Scope
	}
	respondJSON(w, http.StatusOK, resp)
}

func nullableNullTime(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}
