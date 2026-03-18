package api

import (
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

func (s *Server) handleTargetBindings(w http.ResponseWriter, r *http.Request) {
	bindingType := pathVal(r, "binding_type")
	targetName := pathVal(r, "target_name")
	if bindingType == "" || targetName == "" {
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListBindingsFiltered(bindingType, targetName, "", "", limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list target bindings")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for i := range rows {
		item := map[string]any{
			"binding_id":    rows[i].BindingID,
			"binding_type":  rows[i].BindingType,
			"target_name":   rows[i].TargetName,
			"vault_hash":    rows[i].VaultHash,
			"secret_name":   rows[i].SecretName,
			"field_key":     rows[i].FieldKey,
			"ref_canonical": rows[i].RefCanonical,
			"required":      rows[i].Required,
		}
		if entry, err := s.db.GetSecretCatalogByRef(rows[i].RefCanonical); err == nil {
			item = applySecretCatalogMeta(item, entry)
		}
		items = append(items, item)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"binding_type": bindingType,
		"target_name":  targetName,
		"bindings":     items,
		"count":        len(items),
		"total_count":  total,
		"limit":        limit,
		"offset":       offset,
	})
}

func (s *Server) handleTargetBindingsReplace(w http.ResponseWriter, r *http.Request) {
	bindingType := pathVal(r, "binding_type")
	targetName := pathVal(r, "target_name")
	if bindingType == "" || targetName == "" {
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	var req struct {
		Bindings []struct {
			BindingID    string `json:"binding_id"`
			VaultHash    string `json:"vault_hash"`
			SecretName   string `json:"secret_name"`
			FieldKey     string `json:"field_key"`
			RefCanonical string `json:"ref_canonical"`
			Required     *bool  `json:"required"`
		} `json:"bindings"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entries := make([]db.Binding, 0, len(req.Bindings))
	for _, item := range req.Bindings {
		refCanonical := strings.TrimSpace(item.RefCanonical)
		if refCanonical == "" {
			s.respondError(w, http.StatusBadRequest, "ref_canonical is required")
			return
		}
		entry, err := s.db.GetSecretCatalogByRef(refCanonical)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "secret catalog entry not found for "+refCanonical)
			return
		}
		required := true
		if item.Required != nil {
			required = *item.Required
		}
		bindingID := strings.TrimSpace(item.BindingID)
		if bindingID == "" {
			bindingID = crypto.GenerateUUID()
		}
		entries = append(entries, db.Binding{
			BindingID:    bindingID,
			BindingType:  bindingType,
			TargetName:   targetName,
			VaultHash:    strings.TrimSpace(item.VaultHash),
			SecretName:   strings.TrimSpace(item.SecretName),
			FieldKey:     strings.TrimSpace(item.FieldKey),
			RefCanonical: refCanonical,
			Required:     required,
		})
		if entries[len(entries)-1].VaultHash == "" {
			entries[len(entries)-1].VaultHash = entry.VaultHash
		}
		if entries[len(entries)-1].SecretName == "" {
			entries[len(entries)-1].SecretName = entry.SecretName
		}
	}

	if err := s.db.DeleteBindingsByTarget(bindingType, targetName); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to clear existing target bindings")
		return
	}
	for i := range entries {
		if err := s.db.SaveBinding(&entries[i]); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to save target bindings")
			return
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"binding_type": bindingType,
		"target_name":  targetName,
		"bindings":     entries,
		"saved":        len(entries),
	})
}

func (s *Server) handleTargetImpact(w http.ResponseWriter, r *http.Request) {
	bindingType := pathVal(r, "binding_type")
	targetName := pathVal(r, "target_name")
	if bindingType == "" || targetName == "" {
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	rows, err := s.db.ListBindingsByTarget(bindingType, targetName)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load target bindings")
		return
	}
	refs := make([]map[string]any, 0, len(rows))
	for i := range rows {
		ref := map[string]any{
			"binding_id":    rows[i].BindingID,
			"ref_canonical": rows[i].RefCanonical,
			"secret_name":   rows[i].SecretName,
			"field_key":     rows[i].FieldKey,
			"vault_hash":    rows[i].VaultHash,
			"required":      rows[i].Required,
		}
		if entry, err := s.db.GetSecretCatalogByRef(rows[i].RefCanonical); err == nil {
			ref = applySecretCatalogMeta(ref, entry)
		}
		refs = append(refs, ref)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"binding_type": bindingType,
		"target_name":  targetName,
		"refs":         refs,
		"count":        len(refs),
	})
}

func (s *Server) handleTargetBindingsDeleteAll(w http.ResponseWriter, r *http.Request) {
	bindingType := pathVal(r, "binding_type")
	targetName := pathVal(r, "target_name")
	if bindingType == "" || targetName == "" {
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	rows, err := s.db.ListBindingsByTarget(bindingType, targetName)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list target bindings")
		return
	}
	if err := s.db.DeleteBindingsByTarget(bindingType, targetName); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to delete target bindings")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"binding_type": bindingType,
		"target_name":  targetName,
		"deleted":      len(rows),
	})
}

func (s *Server) handleTargetSummary(w http.ResponseWriter, r *http.Request) {
	bindingType := pathVal(r, "binding_type")
	targetName := pathVal(r, "target_name")
	if bindingType == "" || targetName == "" {
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	bindings, total, err := s.db.ListBindingsFiltered(bindingType, targetName, "", "", 20, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list target bindings")
		return
	}
	refs := make([]map[string]any, 0, len(bindings))
	uniqueRefs := map[string]struct{}{}
	vaults := map[string]struct{}{}
	for i := range bindings {
		ref := map[string]any{
			"binding_id":    bindings[i].BindingID,
			"ref_canonical": bindings[i].RefCanonical,
			"secret_name":   bindings[i].SecretName,
			"field_key":     bindings[i].FieldKey,
			"vault_hash":    bindings[i].VaultHash,
			"required":      bindings[i].Required,
		}
		if entry, err := s.db.GetSecretCatalogByRef(bindings[i].RefCanonical); err == nil {
			ref = applySecretCatalogMeta(ref, entry)
		}
		refs = append(refs, ref)
		uniqueRefs[bindings[i].RefCanonical] = struct{}{}
		if bindings[i].VaultHash != "" {
			vaults[bindings[i].VaultHash] = struct{}{}
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"binding_type":      bindingType,
		"target_name":       targetName,
		"bindings":          refs,
		"bindings_count":    len(refs),
		"bindings_total":    total,
		"unique_refs_count": len(uniqueRefs),
		"vaults_count":      len(vaults),
	})
}
