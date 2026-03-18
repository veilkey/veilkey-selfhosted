package hkm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"slices"
	"strings"

	"veilkey-vaultcenter/internal/crypto"
	"veilkey-vaultcenter/internal/db"
)

type agentSecretMeta struct {
	Name        string `json:"name"`
	Ref         string `json:"ref"`
	Token       string `json:"token"`
	Scope       string `json:"scope"`
	Status      string `json:"status"`
	Version     int    `json:"version"`
	FieldsCount int    `json:"fields_count"`
	Fields      []struct {
		Key  string `json:"key"`
		Type string `json:"type"`
	} `json:"fields"`
}

func normalizeFallbackSecretRef(raw string) (ref string, scope string, status string) {
	scope = "TEMP"
	status = "temp"
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", scope, status
	}
	parts := strings.Split(raw, ":")
	if len(parts) == 3 && strings.EqualFold(parts[0], "VK") {
		scope = strings.ToUpper(strings.TrimSpace(parts[1]))
		ref = strings.TrimSpace(parts[2])
		switch scope {
		case "LOCAL", "EXTERNAL":
			status = "active"
		default:
			scope = "TEMP"
			status = "temp"
		}
		return ref, scope, status
	}
	return raw, scope, status
}

func vaultRespFromAgent(a *db.Agent) map[string]any {
	status := "ok"
	if a.BlockedAt != nil {
		status = "blocked"
	} else if a.RotationRequired {
		status = "rotation_required"
	} else if a.RebindRequired {
		status = "rebind_required"
	}
	return map[string]any{
		"node_id":            a.NodeID,
		"vault_node_uuid":    a.NodeID,
		"label":              a.Label,
		"vault_runtime_hash": a.AgentHash,
		"vault_hash":         a.VaultHash,
		"vault_name":         a.VaultName,
		"display_name":       a.VaultName,
		"description":        "",
		"agent_role":         a.AgentRole,
		"host_enabled":       a.HostEnabled,
		"local_enabled":      a.LocalEnabled,
		"tags_json":          "[]",
		"tags":               []string{},
		"vault_id":           httputil.FormatVaultID(a.VaultName, a.VaultHash),
		"managed_paths":      db.DecodeManagedPaths(a.ManagedPaths),
		"key_version":        a.KeyVersion,
		"status":             status,
		"rotation_required":  a.RotationRequired,
		"rebind_required":    a.RebindRequired,
		"blocked":            a.BlockedAt != nil,
		"block_reason":       a.BlockReason,
		"ip":                 a.IP,
		"port":               a.Port,
		"secrets_count":      a.SecretsCount,
		"configs_count":      a.ConfigsCount,
		"version":            a.Version,
		"has_dek":            len(a.DEK) > 0,
		"last_seen":          a.LastSeen.Format("2006-01-02 15:04:05"),
	}
}

func decodeStringArrayJSON(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return values
}

func encodeStringArrayJSON(values []string) string {
	if values == nil {
		values = []string{}
	}
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func applyVaultInventoryMeta(payload map[string]any, inventory *db.VaultInventory) map[string]any {
	if inventory == nil {
		return payload
	}
	payload["display_name"] = inventory.DisplayName
	payload["description"] = inventory.Description
	payload["tags_json"] = inventory.TagsJSON
	payload["tags"] = decodeStringArrayJSON(inventory.TagsJSON)
	return payload
}

func applySecretCatalogMeta(payload map[string]any, entry *db.SecretCatalog) map[string]any {
	if entry == nil {
		return payload
	}
	payload["display_name"] = entry.DisplayName
	payload["description"] = entry.Description
	payload["tags_json"] = entry.TagsJSON
	payload["tags"] = decodeStringArrayJSON(entry.TagsJSON)
	payload["binding_count"] = entry.BindingCount
	payload["usage_count"] = entry.BindingCount
	payload["last_rotated_at"] = entry.LastRotatedAt
	payload["last_revealed_at"] = entry.LastRevealedAt
	return payload
}

func (h *Handler) handleVaultList(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list vaults")
		return
	}

	inventoryRows, _ := h.deps.DB().ListVaultInventory()
	inventoryByNode := map[string]*db.VaultInventory{}
	for i := range inventoryRows {
		row := inventoryRows[i]
		inventoryByNode[row.VaultNodeUUID] = &row
	}

	items := make([]map[string]any, 0, len(agents))
	for i := range agents {
		agent := &agents[i]
		if !agent.LocalEnabled {
			continue
		}
		payload := applyVaultInventoryMeta(vaultRespFromAgent(agent), inventoryByNode[agent.NodeID])
		if statusFilter != "" && payload["status"] != statusFilter {
			continue
		}
		if query != "" {
			haystack := []string{
				strings.ToLower(agent.Label),
				strings.ToLower(agent.AgentHash),
				strings.ToLower(agent.VaultName),
				strings.ToLower(agent.VaultHash),
				strings.ToLower(httputil.FormatVaultID(agent.VaultName, agent.VaultHash)),
			}
			if !slices.ContainsFunc(haystack, func(value string) bool { return strings.Contains(value, query) }) {
				continue
			}
		}
		items = append(items, payload)
	}

	total := len(items)
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	page := items[offset:end]

	respondJSON(w, http.StatusOK, map[string]any{
		"vaults":      page,
		"count":       len(page),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (h *Handler) fetchAgentSecretMeta(agentURL, name string) (*agentSecretMeta, int, []byte, error) {
	resp, err := http.Get(agentURL + "/api/secrets/meta/" + name)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		fallbackResp, err := http.Get(agentURL + "/api/secrets/" + name)
		if err != nil {
			return nil, 0, nil, err
		}
		defer fallbackResp.Body.Close()

		fallbackBody, _ := io.ReadAll(fallbackResp.Body)
		if fallbackResp.StatusCode == http.StatusOK {
			var secret struct {
				Name   string `json:"name"`
				Ref    string `json:"ref"`
				Scope  string `json:"scope"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(fallbackBody, &secret); err == nil && strings.TrimSpace(secret.Ref) != "" {
				ref, scope, status := normalizeFallbackSecretRef(secret.Ref)
				if strings.TrimSpace(secret.Scope) != "" || strings.TrimSpace(secret.Status) != "" {
					scope, status, err = normalizeScopeStatus("VK", secret.Scope, secret.Status, "TEMP")
					if err != nil {
						return nil, 0, nil, err
					}
				}
				resolvedName := secret.Name
				if strings.TrimSpace(resolvedName) == "" {
					resolvedName = name
				}
				return &agentSecretMeta{
					Name:   resolvedName,
					Ref:    ref,
					Scope:  scope,
					Status: status,
				}, http.StatusOK, fallbackBody, nil
			}
		}

		listResp, err := http.Get(agentURL + "/api/secrets")
		if err != nil {
			return nil, fallbackResp.StatusCode, fallbackBody, nil
		}
		defer listResp.Body.Close()

		listBody, _ := io.ReadAll(listResp.Body)
		if listResp.StatusCode == http.StatusOK {
			var listed struct {
				Secrets []struct {
					Name   string `json:"name"`
					Ref    string `json:"ref"`
					Scope  string `json:"scope"`
					Status string `json:"status"`
				} `json:"secrets"`
			}
			if err := json.Unmarshal(listBody, &listed); err == nil {
				for _, item := range listed.Secrets {
					if item.Name != name {
						continue
					}
					ref, scope, status := normalizeFallbackSecretRef(item.Ref)
					if strings.TrimSpace(item.Scope) != "" || strings.TrimSpace(item.Status) != "" {
						scope, status, err = normalizeScopeStatus("VK", item.Scope, item.Status, "TEMP")
						if err != nil {
							return nil, 0, nil, err
						}
					}
					return &agentSecretMeta{
						Name:   item.Name,
						Ref:    ref,
						Scope:  scope,
						Status: status,
					}, http.StatusOK, listBody, nil
				}
			}
		}
		return nil, fallbackResp.StatusCode, fallbackBody, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, body, nil
	}
	var meta agentSecretMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, 0, nil, err
	}
	return &meta, resp.StatusCode, body, nil
}

func normalizeMeta(meta *agentSecretMeta) error {
	var err error
	meta.Scope, meta.Status, err = normalizeScopeStatus("VK", meta.Scope, meta.Status, "TEMP")
	if err != nil {
		return err
	}
	meta.Token = "VK:" + meta.Scope + ":" + meta.Ref
	return nil
}

func (h *Handler) handleVaultGet(w http.ResponseWriter, r *http.Request) {
	agent, err := h.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	if !agent.LocalEnabled {
		respondError(w, http.StatusNotFound, "vault not found")
		return
	}
	if err := validateAgentAvailability(agent); err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	inventory, _ := h.deps.DB().GetVaultInventoryByNodeID(agent.NodeID)
	respondJSON(w, http.StatusOK, applyVaultInventoryMeta(vaultRespFromAgent(agent), inventory))
}

func (h *Handler) handleVaultAudit(w http.ResponseWriter, r *http.Request) {
	agent, err := h.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	if err := validateAgentAvailability(agent); err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListAuditEventsForVault(agent.NodeID, agent.AgentHash, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list vault audit events")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": agent.AgentHash,
		"vault_node_uuid":    agent.NodeID,
		"events":             rows,
		"count":              len(rows),
		"total_count":        total,
		"limit":              limit,
		"offset":             offset,
	})
}

func (h *Handler) handleVaultKeys(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentSecrets(w, r)
}

func (h *Handler) handleVaultKeyMeta(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	resp := map[string]any{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"version":            meta.Version,
		"fields":             meta.Fields,
		"fields_count":       meta.FieldsCount,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	}
	entry, _ := h.deps.DB().GetSecretCatalogByRef(meta.Token)
	respondJSON(w, http.StatusOK, applySecretCatalogMeta(resp, entry))
}

func (h *Handler) handleVaultKeyUsage(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListBindingsByRefFiltered(meta.Token, "", limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list key usage")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"usage_count":        total,
		"bindings":           rows,
		"count":              len(rows),
		"total_count":        total,
		"limit":              limit,
		"offset":             offset,
	})
}

func (h *Handler) handleVaultKeyBindings(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListBindingsByRefFiltered(meta.Token, "", limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list key bindings")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"bindings":           rows,
		"count":              len(rows),
		"total_count":        total,
		"limit":              limit,
		"offset":             offset,
	})
}

func (h *Handler) handleVaultKeyBindingSave(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	var raw map[string]json.RawMessage
	if err := httputil.DecodeJSON(r, &raw); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req struct {
		BindingID   string
		BindingType string
		TargetName  string
		FieldKey    string
		Required    bool
	}
	if value := raw["binding_id"]; len(value) > 0 {
		_ = json.Unmarshal(value, &req.BindingID)
	}
	if value := raw["binding_type"]; len(value) > 0 {
		_ = json.Unmarshal(value, &req.BindingType)
	}
	if value := raw["target_name"]; len(value) > 0 {
		_ = json.Unmarshal(value, &req.TargetName)
	}
	if value := raw["field_key"]; len(value) > 0 {
		_ = json.Unmarshal(value, &req.FieldKey)
	}
	req.BindingType = strings.TrimSpace(req.BindingType)
	req.TargetName = strings.TrimSpace(req.TargetName)
	req.FieldKey = strings.TrimSpace(req.FieldKey)
	if req.BindingType == "" || req.TargetName == "" {
		respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	required := true
	if value := raw["required"]; len(value) > 0 {
		if err := json.Unmarshal(value, &req.Required); err != nil {
			respondError(w, http.StatusBadRequest, "required must be boolean")
			return
		}
		required = req.Required
	}
	bindingID := strings.TrimSpace(req.BindingID)
	if bindingID == "" {
		bindingID = crypto.GenerateUUID()
	}

	entry := db.Binding{
		BindingID:    bindingID,
		BindingType:  req.BindingType,
		TargetName:   req.TargetName,
		VaultHash:    agent.VaultHash,
		SecretName:   name,
		FieldKey:     req.FieldKey,
		RefCanonical: meta.Token,
		Required:     required,
	}
	if err := h.deps.DB().SaveBinding(&entry); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save binding")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"binding_id":         entry.BindingID,
		"binding_type":       entry.BindingType,
		"target_name":        entry.TargetName,
		"field_key":          entry.FieldKey,
		"required":           entry.Required,
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (h *Handler) handleVaultKeyBindingsReplace(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, meta, ok := h.lookupVaultKeyForBindingWrite(w, hashOrLabel, name)
	if !ok {
		return
	}

	var req struct {
		Bindings []struct {
			BindingID   string `json:"binding_id"`
			BindingType string `json:"binding_type"`
			TargetName  string `json:"target_name"`
			FieldKey    string `json:"field_key"`
			Required    *bool  `json:"required"`
		} `json:"bindings"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entries := make([]db.Binding, 0, len(req.Bindings))
	for _, item := range req.Bindings {
		bindingType := strings.TrimSpace(item.BindingType)
		targetName := strings.TrimSpace(item.TargetName)
		fieldKey := strings.TrimSpace(item.FieldKey)
		if bindingType == "" || targetName == "" {
			respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
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
			VaultHash:    agent.VaultHash,
			SecretName:   name,
			FieldKey:     fieldKey,
			RefCanonical: meta.Token,
			Required:     required,
		})
	}

	existing, _, err := h.deps.DB().ListBindingsByRefFiltered(meta.Token, "", 0, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load existing bindings")
		return
	}
	for _, row := range existing {
		if err := h.deps.DB().DeleteBinding(row.BindingID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to clear existing bindings")
			return
		}
	}
	for i := range entries {
		if err := h.deps.DB().SaveBinding(&entries[i]); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to save replacement bindings")
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"bindings":           entries,
		"saved":              len(entries),
	})
}

func (h *Handler) handleVaultKeyBindingsDeleteAll(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, meta, ok := h.lookupVaultKeyForBindingWrite(w, hashOrLabel, name)
	if !ok {
		return
	}

	rows, _, err := h.deps.DB().ListBindingsByRefFiltered(meta.Token, "", 0, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	for _, row := range rows {
		if err := h.deps.DB().DeleteBinding(row.BindingID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to delete bindings")
			return
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"deleted":            len(rows),
	})
}

func (h *Handler) handleVaultKeyAudit(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := httputil.ParseListWindow(r)
	if errMsg != "" {
		respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := h.deps.DB().ListAuditEventsLimited("secret", meta.Token, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list key audit events")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"events":             rows,
		"count":              len(rows),
		"total_count":        total,
		"limit":              limit,
		"offset":             offset,
	})
}

func (h *Handler) handleVaultKeyBindingDelete(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	bindingID := httputil.PathVal(r, "binding_id")
	if bindingID == "" {
		respondError(w, http.StatusBadRequest, "binding_id is required")
		return
	}
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	row, err := h.deps.DB().GetBinding(bindingID)
	if err != nil {
		respondError(w, http.StatusNotFound, "binding not found")
		return
	}
	if row.RefCanonical != meta.Token {
		respondError(w, http.StatusNotFound, "binding not found for key")
		return
	}
	if err := h.deps.DB().DeleteBinding(bindingID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete binding")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"deleted":            bindingID,
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (h *Handler) lookupVaultKeyForBindingWrite(w http.ResponseWriter, hashOrLabel, name string) (*agentInfo, *agentSecretMeta, bool) {
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return nil, nil, false
	}
	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return nil, nil, false
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return nil, nil, false
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return nil, nil, false
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return nil, nil, false
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)
	return agent, meta, true
}

func (h *Handler) handleVaultKeyGet(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentGetSecret(w, r)
}

func (h *Handler) handleVaultPatch(w http.ResponseWriter, r *http.Request) {
	agent, err := h.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	var req struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.deps.DB().UpdateVaultInventoryMeta(agent.NodeID, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), encodeStringArrayJSON(req.Tags)); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update vault metadata")
		return
	}
	inventory, _ := h.deps.DB().GetVaultInventoryByNodeID(agent.NodeID)
	respondJSON(w, http.StatusOK, applyVaultInventoryMeta(vaultRespFromAgent(agent), inventory))
}

func (h *Handler) handleVaultKeyMetaPatch(w http.ResponseWriter, r *http.Request) {
	_, meta, ok := h.lookupVaultKeyForBindingWrite(w, r.PathValue("vault"), r.PathValue("name"))
	if !ok {
		return
	}
	var req struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.deps.DB().UpdateSecretCatalogMeta(meta.Token, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), encodeStringArrayJSON(req.Tags)); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update key metadata")
		return
	}
	entry, err := h.deps.DB().GetSecretCatalogByRef(meta.Token)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load updated key metadata")
		return
	}
	resp := map[string]any{
		"name":         meta.Name,
		"ref":          meta.Ref,
		"token":        meta.Token,
		"scope":        meta.Scope,
		"status":       meta.Status,
		"version":      meta.Version,
		"fields":       meta.Fields,
		"fields_count": meta.FieldsCount,
	}
	respondJSON(w, http.StatusOK, applySecretCatalogMeta(resp, entry))
}

func (h *Handler) handleVaultKeySummary(w http.ResponseWriter, r *http.Request) {
	agent, meta, ok := h.lookupVaultKeyForBindingWrite(w, r.PathValue("vault"), r.PathValue("name"))
	if !ok {
		return
	}
	entry, _ := h.deps.DB().GetSecretCatalogByRef(meta.Token)
	bindings, totalBindings, err := h.deps.DB().ListBindingsByRefFiltered(meta.Token, "", 20, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	events, totalEvents, err := h.deps.DB().ListAuditEventsLimited("secret", meta.Token, 10, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}
	key := map[string]any{
		"name":               meta.Name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"version":            meta.Version,
		"fields":             meta.Fields,
		"fields_count":       meta.FieldsCount,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	}
	key = applySecretCatalogMeta(key, entry)
	vault, err := h.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}
	inventory, _ := h.deps.DB().GetVaultInventoryByNodeID(vault.NodeID)
	usageCount := int64(totalBindings)
	if entry != nil {
		usageCount = int64(entry.BindingCount)
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"vault":              applyVaultInventoryMeta(vaultRespFromAgent(vault), inventory),
		"key":                key,
		"bindings":           bindings,
		"bindings_count":     len(bindings),
		"bindings_total":     totalBindings,
		"usage_count":        usageCount,
		"recent_audit":       events,
		"recent_audit_count": len(events),
		"recent_audit_total": totalEvents,
	})
}

func (h *Handler) handleVaultKeySave(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentSaveSecret(w, r)
}

func (h *Handler) handleVaultKeyUpdate(w http.ResponseWriter, r *http.Request) {
	name := httputil.PathVal(r, "name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		respondError(w, http.StatusBadRequest, "value is required")
		return
	}
	if bodyName := strings.TrimSpace(req.Name); bodyName != "" && bodyName != name {
		respondError(w, http.StatusBadRequest, "body name must match path name")
		return
	}
	r.SetPathValue("agent", r.PathValue("vault"))
	payload, _ := json.Marshal(map[string]string{
		"name":  name,
		"value": req.Value,
	})
	r.Body = ioNopCloser{bytes.NewReader(payload)}
	r.ContentLength = int64(len(payload))
	r.Header.Set("Content-Type", "application/json")
	h.handleAgentSaveSecret(w, r)
}

func (h *Handler) handleVaultKeyDelete(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentDeleteSecret(w, r)
}

func (h *Handler) handleVaultKeyFields(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := h.findAgent(hashOrLabel)
	if err != nil {
		h.respondAgentLookupError(w, err)
		return
	}

	meta, statusCode, body, err := h.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = h.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"ref":                meta.Ref,
		"token":              meta.Token,
		"scope":              meta.Scope,
		"status":             meta.Status,
		"fields":             meta.Fields,
		"fields_count":       meta.FieldsCount,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (h *Handler) handleVaultKeyFieldsUpdate(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentSaveSecretFields(w, r)
}

func (h *Handler) handleVaultKeyFieldGet(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentGetSecretField(w, r)
}

func (h *Handler) handleVaultKeyFieldUpdate(w http.ResponseWriter, r *http.Request) {
	name := httputil.PathVal(r, "name")
	fieldKey := httputil.PathVal(r, "field")
	if name == "" || fieldKey == "" {
		respondError(w, http.StatusBadRequest, "name and field are required")
		return
	}
	var req struct {
		Key   string `json:"key"`
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		respondError(w, http.StatusBadRequest, "value is required")
		return
	}
	if bodyKey := strings.TrimSpace(req.Key); bodyKey != "" && bodyKey != fieldKey {
		respondError(w, http.StatusBadRequest, "body key must match path field")
		return
	}
	r.SetPathValue("agent", r.PathValue("vault"))
	payload, _ := json.Marshal(map[string]any{
		"fields": []map[string]string{{
			"key":   fieldKey,
			"type":  req.Type,
			"value": req.Value,
		}},
	})
	r.Body = ioNopCloser{bytes.NewReader(payload)}
	r.ContentLength = int64(len(payload))
	r.Header.Set("Content-Type", "application/json")
	h.handleAgentSaveSecretFields(w, r)
}

func (h *Handler) handleVaultKeyFieldDelete(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	h.handleAgentDeleteSecretField(w, r)
}

func (h *Handler) handleVaultKeyActivate(w http.ResponseWriter, r *http.Request) {
	respondError(w, http.StatusNotImplemented, "key activation must be performed by the local vault lifecycle boundary and mirrored back via tracked ref sync")
}

type ioNopCloser struct {
	*bytes.Reader
}

func (n ioNopCloser) Close() error { return nil }
