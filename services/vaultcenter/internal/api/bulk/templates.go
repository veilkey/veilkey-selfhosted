package bulk

import (
	"net/http"
	"regexp"
	"sort"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

// ---------------------------------------------------------------------------
// Placeholder pattern and helpers
// ---------------------------------------------------------------------------

var bulkApplyPlaceholderPattern = regexp.MustCompile(`\{\{\s*(VK|VE)\.([A-Za-z0-9_]+)\s*\}\}`)

func bulkApplyTemplateResponse(tmpl *bulkApplyTemplateRecord) map[string]any {
	if tmpl == nil {
		return map[string]any{}
	}
	return map[string]any{
		"template_id":        tmpl.TemplateID,
		"vault_runtime_hash": tmpl.VaultRuntimeHash,
		"name":               tmpl.Name,
		"format":             tmpl.Format,
		"target_path":        tmpl.TargetPath,
		"body":               tmpl.Body,
		"hook":               tmpl.Hook,
		"enabled":            tmpl.Enabled,
		"created_at":         tmpl.CreatedAt,
		"updated_at":         tmpl.UpdatedAt,
		"validation_status":  tmpl.ValidationStatus,
		"validation_message": tmpl.ValidationMessage,
	}
}

func bulkApplyPlaceholders(body string) []map[string]string {
	matches := bulkApplyPlaceholderPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return []map[string]string{}
	}
	seen := map[string]struct{}{}
	items := make([]map[string]string, 0, len(matches))
	for _, match := range matches {
		token := strings.TrimSpace(match[0])
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])
		key := kind + ":" + name
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, map[string]string{
			"kind":  kind,
			"name":  name,
			"token": token,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i]["kind"] == items[j]["kind"] {
			return items[i]["name"] < items[j]["name"]
		}
		return items[i]["kind"] < items[j]["kind"]
	})
	return items
}

func renderBulkApplyPreview(body string) string {
	return bulkApplyPlaceholderPattern.ReplaceAllStringFunc(body, func(token string) string {
		match := bulkApplyPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		return "<" + strings.TrimSpace(match[1]) + "." + strings.TrimSpace(match[2]) + ">"
	})
}

func isSensitiveBulkApplyValue(kind, name string) bool {
	if strings.EqualFold(strings.TrimSpace(kind), db.RefFamilyVE) {
		return false
	}
	upper := strings.ToUpper(strings.TrimSpace(name))
	for _, safeNeedle := range []string{"ENDPOINT", "URL", "DOMAIN", "COLOR", "TEXT", "SCOPE", "ENABLE", "HOST", "PORT", "NAME", "CLIENT_ID", "BUTTON"} {
		if strings.Contains(upper, safeNeedle) {
			return false
		}
	}
	for _, needle := range []string{"PASSWORD", "SECRET", "TOKEN", "CREDENTIAL", "PRIVATE", "PASS", "KEY"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func maskBulkApplyValue(kind, name, value string) string {
	if !isSensitiveBulkApplyValue(kind, name) {
		return value
	}
	return "***"
}

// renderResolvedBulkApplyPreview renders body with placeholder values resolved
// via Deps.ResolveTemplateValue. Unresolvable placeholders are shown as
// <KIND.NAME>. The vaultHash is used to scope the resolution.
func (h *Handler) renderResolvedBulkApplyPreview(vaultHash, body string) string {
	// If no agent is reachable we still return a best-effort preview.
	return bulkApplyPlaceholderPattern.ReplaceAllStringFunc(body, func(token string) string {
		match := bulkApplyPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])

		// Map VK→"secret", VE→"config" to match the Deps interface contract.
		depKind := "secret"
		if kind == db.RefFamilyVE {
			depKind = "config"
		}

		value, ok := h.deps.ResolveTemplateValue(vaultHash, depKind, name)
		if !ok {
			return "<" + kind + "." + name + ">"
		}
		return maskBulkApplyValue(kind, name, value)
	})
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleBulkApplyTemplates(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	if vaultHash == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := h.listBulkApplyTemplateRecords(vaultHash)
		if err != nil {
			httputil.RespondError(w, http.StatusInternalServerError, "failed to list bulk apply templates")
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for i := range rows {
			items = append(items, bulkApplyTemplateResponse(&rows[i]))
		}
		httputil.RespondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"templates":          items,
			"count":              len(items),
		})
	case http.MethodPost:
		var req bulkApplyTemplatePayload
		if err := httputil.DecodeJSON(r, &req); err != nil {
			httputil.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		tmpl, err := h.saveBulkApplyTemplateFile(vaultHash, "", &req)
		if err != nil {
			httputil.RespondError(w, http.StatusBadRequest, "invalid request")
			return
		}
		httputil.RespondJSON(w, http.StatusOK, bulkApplyTemplateResponse(tmpl))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleBulkApplyTemplate(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	name := httputil.PathVal(r, "name")
	if vaultHash == "" || name == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault and name are required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		tmpl, err := h.loadBulkApplyTemplateRecord(vaultHash, name)
		if err != nil {
			httputil.RespondError(w, http.StatusNotFound, "not found")
			return
		}
		httputil.RespondJSON(w, http.StatusOK, bulkApplyTemplateResponse(tmpl))
	case http.MethodPut:
		var req bulkApplyTemplatePayload
		if err := httputil.DecodeJSON(r, &req); err != nil {
			httputil.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		current, err := h.saveBulkApplyTemplateFile(vaultHash, name, &req)
		if err != nil {
			httputil.RespondError(w, http.StatusBadRequest, "invalid request")
			return
		}
		httputil.RespondJSON(w, http.StatusOK, bulkApplyTemplateResponse(current))
	case http.MethodDelete:
		if err := h.deleteBulkApplyTemplateFile(vaultHash, name); err != nil {
			httputil.RespondError(w, http.StatusNotFound, "not found")
			return
		}
		httputil.RespondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"name":               name,
			"deleted":            true,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleBulkApplyTemplatePreview(w http.ResponseWriter, r *http.Request) {
	vaultHash := httputil.PathVal(r, "vault")
	name := httputil.PathVal(r, "name")
	if vaultHash == "" || name == "" {
		httputil.RespondError(w, http.StatusBadRequest, "vault and name are required")
		return
	}
	tmpl, err := h.loadBulkApplyTemplateRecord(vaultHash, name)
	if err != nil {
		httputil.RespondError(w, http.StatusNotFound, "not found")
		return
	}
	if tmpl.ValidationStatus != "valid" {
		httputil.RespondError(w, http.StatusBadRequest, "template definition is broken")
		return
	}
	httputil.RespondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"name":               name,
		"format":             tmpl.Format,
		"target_path":        tmpl.TargetPath,
		"hook":               tmpl.Hook,
		"enabled":            tmpl.Enabled,
		"placeholders":       bulkApplyPlaceholders(tmpl.Body),
		"preview":            h.renderResolvedBulkApplyPreview(vaultHash, tmpl.Body),
	})
}
