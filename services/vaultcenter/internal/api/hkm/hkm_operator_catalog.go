package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

func secretCatalogPayload(entry db.SecretCatalog) map[string]any {
	return map[string]any{
		"secret_canonical_id": entry.SecretCanonicalID,
		"secret_name":         entry.SecretName,
		"display_name":        entry.DisplayName,
		"description":         entry.Description,
		"tags_json":           entry.TagsJSON,
		"class":               entry.Class,
		"scope":               entry.Scope,
		"status":              entry.Status,
		"vault_node_uuid":     entry.VaultNodeUUID,
		"vault_runtime_hash":  entry.VaultRuntimeHash,
		"vault_hash":          entry.VaultHash,
		"ref_canonical":       entry.RefCanonical,
		"fields_present_json": entry.FieldsPresentJSON,
		"binding_count":       entry.BindingCount,
		"usage_count":         entry.BindingCount,
		"last_rotated_at":     entry.LastRotatedAt,
		"last_revealed_at":    entry.LastRevealedAt,
		"updated_at":          entry.UpdatedAt,
	}
}

func (h *Handler) handleVaultInventory(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListVaultInventoryFiltered(
		r.URL.Query().Get("status"),
		r.URL.Query().Get("vault_hash"),
		limit,
		offset,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list vault inventory")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"vaults":      rows,
		"count":       len(rows),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) handleSecretCatalogList(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListSecretCatalogFiltered(
		r.URL.Query().Get("vault_hash"),
		r.URL.Query().Get("class"),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("q"),
		limit,
		offset,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list secret catalog")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, secretCatalogPayload(row))
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"secrets":     items,
		"count":       len(items),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) handleSecretCatalogGet(w http.ResponseWriter, r *http.Request) {
	refCanonical := r.PathValue("ref")
	if refCanonical == "" {
		respondError(w, http.StatusBadRequest, "ref is required")
		return
	}

	entry, err := h.deps.DB().GetSecretCatalogByRef(refCanonical)
	if err != nil {
		respondError(w, http.StatusNotFound, "secret catalog entry not found")
		return
	}
	respondJSON(w, http.StatusOK, secretCatalogPayload(*entry))
}

func (h *Handler) handleBindingsList(w http.ResponseWriter, r *http.Request) {
	bindingType := r.URL.Query().Get("binding_type")
	targetName := r.URL.Query().Get("target_name")
	refCanonical := r.URL.Query().Get("ref_canonical")

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	var (
		rows  []db.Binding
		total int64
		err   error
	)
	switch {
	case refCanonical != "":
		rows, total, err = h.deps.DB().ListBindingsByRefFiltered(
			refCanonical,
			r.URL.Query().Get("vault_hash"),
			limit,
			offset,
		)
	case bindingType != "" && targetName != "":
		rows, total, err = h.deps.DB().ListBindingsFiltered(
			bindingType,
			targetName,
			r.URL.Query().Get("vault_hash"),
			refCanonical,
			limit,
			offset,
		)
	default:
		respondError(w, http.StatusBadRequest, "either ref_canonical or binding_type and target_name are required")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"bindings":    rows,
		"count":       len(rows),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) handleAuditEventsList(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")
	if entityType == "" || entityID == "" {
		respondError(w, http.StatusBadRequest, "entity_type and entity_id are required")
		return
	}

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListAuditEventsLimited(entityType, entityID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"events":      rows,
		"count":       len(rows),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}
