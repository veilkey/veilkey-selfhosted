package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"veilkey-vaultcenter/internal/crypto"
)

var bulkApplyPlaceholderPattern = regexp.MustCompile(`\{\{\s*(VK|VE)\.([A-Za-z0-9_]+)\s*\}\}`)

type bulkApplyTemplatePayload struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	TargetPath string `json:"target_path"`
	Body       string `json:"body"`
	Hook       string `json:"hook"`
	Enabled    *bool  `json:"enabled"`
}

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
	if strings.EqualFold(strings.TrimSpace(kind), "VE") {
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

func (s *Server) resolveBulkApplySecretValue(agent *agentInfo, name string) (string, bool) {
	if agent == nil {
		return "", false
	}
	meta, statusCode, _, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil || statusCode != http.StatusOK || meta == nil {
		return "", false
	}
	if err := normalizeMeta(meta); err != nil {
		return "", false
	}
	if meta.Ref == "" {
		return "", false
	}
	cipher, err := s.fetchAgentCiphertext(agent.URL(), meta.Ref)
	if err == nil {
		agentDEK, err := s.decryptAgentDEK(agent.DEK, agent.DEKNonce)
		if err == nil {
			plaintext, decErr := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
			if decErr == nil {
				return string(plaintext), true
			}
		}
	}
	resolved, err := s.fetchAgentResolvedValue(agent.URL(), meta.Token)
	if err != nil || resolved == nil {
		return "", false
	}
	return resolved.Value, true
}

func (s *Server) resolveBulkApplyConfigValue(agent *agentInfo, key string) (string, bool) {
	if agent == nil {
		return "", false
	}
	resp, err := http.Get(agent.URL() + "/api/configs/" + key)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", false
	}
	value, _ := data["value"].(string)
	return value, strings.TrimSpace(value) != ""
}

func (s *Server) renderResolvedBulkApplyPreview(vaultHash, body string) string {
	agent, err := s.findAgent(vaultHash)
	if err != nil {
		return renderBulkApplyPreview(body)
	}
	return bulkApplyPlaceholderPattern.ReplaceAllStringFunc(body, func(token string) string {
		match := bulkApplyPlaceholderPattern.FindStringSubmatch(token)
		if len(match) != 3 {
			return token
		}
		kind := strings.TrimSpace(match[1])
		name := strings.TrimSpace(match[2])
		var (
			value string
			ok    bool
		)
		if kind == "VK" {
			value, ok = s.resolveBulkApplySecretValue(agent, name)
		} else {
			value, ok = s.resolveBulkApplyConfigValue(agent, name)
		}
		if !ok {
			return "<" + kind + "." + name + ">"
		}
		return maskBulkApplyValue(kind, name, value)
	})
}

func (s *Server) handleBulkApplyTemplates(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	if vaultHash == "" {
		s.respondError(w, http.StatusBadRequest, "vault is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.listBulkApplyTemplateRecords(vaultHash)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to list bulk apply templates")
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for i := range rows {
			items = append(items, bulkApplyTemplateResponse(&rows[i]))
		}
		s.respondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"templates":          items,
			"count":              len(items),
		})
	case http.MethodPost:
		var req bulkApplyTemplatePayload
		if err := decodeRequestJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		tmpl, err := s.saveBulkApplyTemplateFile(vaultHash, "", &req)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, bulkApplyTemplateResponse(tmpl))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleBulkApplyTemplate(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	name := pathVal(r, "name")
	if vaultHash == "" || name == "" {
		s.respondError(w, http.StatusBadRequest, "vault and name are required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		tmpl, err := s.loadBulkApplyTemplateRecord(vaultHash, name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, bulkApplyTemplateResponse(tmpl))
	case http.MethodPut:
		var req bulkApplyTemplatePayload
		if err := decodeRequestJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		current, err := s.saveBulkApplyTemplateFile(vaultHash, name, &req)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, bulkApplyTemplateResponse(current))
	case http.MethodDelete:
		if err := s.deleteBulkApplyTemplateFile(vaultHash, name); err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{
			"vault_runtime_hash": vaultHash,
			"name":               name,
			"deleted":            true,
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleBulkApplyTemplatePreview(w http.ResponseWriter, r *http.Request) {
	vaultHash := pathVal(r, "vault")
	name := pathVal(r, "name")
	if vaultHash == "" || name == "" {
		s.respondError(w, http.StatusBadRequest, "vault and name are required")
		return
	}
	tmpl, err := s.loadBulkApplyTemplateRecord(vaultHash, name)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if tmpl.ValidationStatus != "valid" {
		s.respondError(w, http.StatusBadRequest, "template definition is broken")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": vaultHash,
		"name":               name,
		"format":             tmpl.Format,
		"target_path":        tmpl.TargetPath,
		"hook":               tmpl.Hook,
		"enabled":            tmpl.Enabled,
		"placeholders":       bulkApplyPlaceholders(tmpl.Body),
		"preview":            s.renderResolvedBulkApplyPreview(vaultHash, tmpl.Body),
	})
}
