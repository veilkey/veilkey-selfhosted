package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
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
		"vault_id":           formatVaultID(a.VaultName, a.VaultHash),
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

func (s *Server) handleVaultList(w http.ResponseWriter, r *http.Request) {
	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	agents, err := s.db.ListAgents()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list vaults")
		return
	}

	inventoryRows, _ := s.db.ListVaultInventory()
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
				strings.ToLower(formatVaultID(agent.VaultName, agent.VaultHash)),
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

	s.respondJSON(w, http.StatusOK, map[string]any{
		"vaults":      page,
		"count":       len(page),
		"total_count": total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (s *Server) fetchAgentSecretMeta(agentURL, name string) (*agentSecretMeta, int, []byte, error) {
	resp, err := s.httpClient.Get(agentURL + "/api/secrets/meta/" + name)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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

func (s *Server) handleVaultGet(w http.ResponseWriter, r *http.Request) {
	agent, err := s.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	if !agent.LocalEnabled {
		s.respondError(w, http.StatusNotFound, "vault not found")
		return
	}
	if err := validateAgentAvailability(agent); err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	inventory, _ := s.db.GetVaultInventoryByNodeID(agent.NodeID)
	s.respondJSON(w, http.StatusOK, applyVaultInventoryMeta(vaultRespFromAgent(agent), inventory))
}

func (s *Server) handleVaultAudit(w http.ResponseWriter, r *http.Request) {
	agent, err := s.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	if err := validateAgentAvailability(agent); err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListAuditEventsLimited("vault", agent.NodeID, limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list vault audit events")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"vault_runtime_hash": agent.AgentHash,
		"vault_node_uuid":    agent.NodeID,
		"events":             rows,
		"count":              len(rows),
		"total_count":        total,
		"limit":              limit,
		"offset":             offset,
	})
}

func (s *Server) handleVaultKeys(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentSecrets(w, r)
}

func (s *Server) handleVaultKeyMeta(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

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
	entry, _ := s.db.GetSecretCatalogByRef(meta.Token)
	s.respondJSON(w, http.StatusOK, applySecretCatalogMeta(resp, entry))
}

func (s *Server) handleVaultKeyUsage(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListBindingsByRefFiltered(meta.Token, "", limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list key usage")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeyBindings(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListBindingsByRefFiltered(meta.Token, "", limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list key bindings")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeyBindingSave(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
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
		s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
		return
	}
	required := true
	if value := raw["required"]; len(value) > 0 {
		if err := json.Unmarshal(value, &req.Required); err != nil {
			s.respondError(w, http.StatusBadRequest, "required must be boolean")
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
	if err := s.db.SaveBinding(&entry); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save binding")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeyBindingsReplace(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, meta, ok := s.lookupVaultKeyForBindingWrite(w, hashOrLabel, name)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	entries := make([]db.Binding, 0, len(req.Bindings))
	for _, item := range req.Bindings {
		bindingType := strings.TrimSpace(item.BindingType)
		targetName := strings.TrimSpace(item.TargetName)
		fieldKey := strings.TrimSpace(item.FieldKey)
		if bindingType == "" || targetName == "" {
			s.respondError(w, http.StatusBadRequest, "binding_type and target_name are required")
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

	existing, _, err := s.db.ListBindingsByRefFiltered(meta.Token, "", 0, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load existing bindings")
		return
	}
	for _, row := range existing {
		if err := s.db.DeleteBinding(row.BindingID); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to clear existing bindings")
			return
		}
	}
	for i := range entries {
		if err := s.db.SaveBinding(&entries[i]); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to save replacement bindings")
			return
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"bindings":           entries,
		"saved":              len(entries),
	})
}

func (s *Server) handleVaultKeyBindingsDeleteAll(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, meta, ok := s.lookupVaultKeyForBindingWrite(w, hashOrLabel, name)
	if !ok {
		return
	}

	rows, _, err := s.db.ListBindingsByRefFiltered(meta.Token, "", 0, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	for _, row := range rows {
		if err := s.db.DeleteBinding(row.BindingID); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to delete bindings")
			return
		}
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
		"deleted":            len(rows),
	})
}

func (s *Server) handleVaultKeyAudit(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	limit, offset, errMsg := parseListWindow(r)
	if errMsg != "" {
		s.respondError(w, http.StatusBadRequest, errMsg)
		return
	}
	rows, total, err := s.db.ListAuditEventsLimited("secret", meta.Token, limit, offset)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list key audit events")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeyBindingDelete(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	bindingID := strings.TrimSpace(r.PathValue("binding_id"))
	if bindingID == "" {
		s.respondError(w, http.StatusBadRequest, "binding_id is required")
		return
	}
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	row, err := s.db.GetBinding(bindingID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "binding not found")
		return
	}
	if row.RefCanonical != meta.Token {
		s.respondError(w, http.StatusNotFound, "binding not found for key")
		return
	}
	if err := s.db.DeleteBinding(bindingID); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to delete binding")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
		"deleted":            bindingID,
		"name":               name,
		"token":              meta.Token,
		"vault":              agent.Label,
		"vault_runtime_hash": agent.AgentHash,
	})
}

func (s *Server) lookupVaultKeyForBindingWrite(w http.ResponseWriter, hashOrLabel, name string) (*agentInfo, *agentSecretMeta, bool) {
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return nil, nil, false
	}
	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return nil, nil, false
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return nil, nil, false
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return nil, nil, false
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return nil, nil, false
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)
	return agent, meta, true
}

func (s *Server) handleVaultKeyGet(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentGetSecret(w, r)
}

func (s *Server) handleVaultPatch(w http.ResponseWriter, r *http.Request) {
	agent, err := s.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	var req struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.db.UpdateVaultInventoryMeta(agent.NodeID, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), encodeStringArrayJSON(req.Tags)); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to update vault metadata")
		return
	}
	inventory, _ := s.db.GetVaultInventoryByNodeID(agent.NodeID)
	s.respondJSON(w, http.StatusOK, applyVaultInventoryMeta(vaultRespFromAgent(agent), inventory))
}

func (s *Server) handleVaultKeyMetaPatch(w http.ResponseWriter, r *http.Request) {
	_, meta, ok := s.lookupVaultKeyForBindingWrite(w, r.PathValue("vault"), r.PathValue("name"))
	if !ok {
		return
	}
	var req struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.db.UpdateSecretCatalogMeta(meta.Token, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), encodeStringArrayJSON(req.Tags)); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to update key metadata")
		return
	}
	entry, err := s.db.GetSecretCatalogByRef(meta.Token)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load updated key metadata")
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
	s.respondJSON(w, http.StatusOK, applySecretCatalogMeta(resp, entry))
}

func (s *Server) handleVaultKeySummary(w http.ResponseWriter, r *http.Request) {
	agent, meta, ok := s.lookupVaultKeyForBindingWrite(w, r.PathValue("vault"), r.PathValue("name"))
	if !ok {
		return
	}
	entry, _ := s.db.GetSecretCatalogByRef(meta.Token)
	bindings, totalBindings, err := s.db.ListBindingsByRefFiltered(meta.Token, "", 20, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	events, totalEvents, err := s.db.ListAuditEventsLimited("secret", meta.Token, 10, 0)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list audit events")
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
	vault, err := s.findAgentRecord(r.PathValue("vault"))
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}
	inventory, _ := s.db.GetVaultInventoryByNodeID(vault.NodeID)
	usageCount := int64(totalBindings)
	if entry != nil {
		usageCount = int64(entry.BindingCount)
	}
	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeySave(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentSaveSecret(w, r)
}

func (s *Server) handleVaultKeyUpdate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		s.respondError(w, http.StatusBadRequest, "value is required")
		return
	}
	if bodyName := strings.TrimSpace(req.Name); bodyName != "" && bodyName != name {
		s.respondError(w, http.StatusBadRequest, "body name must match path name")
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
	s.handleAgentSaveSecret(w, r)
}

func (s *Server) handleVaultKeyDelete(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentDeleteSecret(w, r)
}

func (s *Server) handleVaultKeyFields(w http.ResponseWriter, r *http.Request) {
	hashOrLabel := r.PathValue("vault")
	name := r.PathValue("name")
	agent, err := s.findAgent(hashOrLabel)
	if err != nil {
		s.respondAgentLookupError(w, err)
		return
	}

	meta, statusCode, body, err := s.fetchAgentSecretMeta(agent.URL(), name)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, "agent unreachable: "+err.Error())
		return
	}
	if statusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body)
		return
	}
	if meta.Ref == "" {
		s.respondError(w, http.StatusInternalServerError, "secret has no ref")
		return
	}
	if err := normalizeMeta(meta); err != nil {
		s.respondError(w, http.StatusBadGateway, "agent returned unsupported secret scope: "+err.Error())
		return
	}
	_ = s.upsertTrackedRef(meta.Token, agent.KeyVersion, meta.Status, agent.AgentHash)

	s.respondJSON(w, http.StatusOK, map[string]any{
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

func (s *Server) handleVaultKeyFieldsUpdate(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentSaveSecretFields(w, r)
}

func (s *Server) handleVaultKeyFieldGet(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentGetSecretField(w, r)
}

func (s *Server) handleVaultKeyFieldUpdate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	fieldKey := strings.TrimSpace(r.PathValue("field"))
	if name == "" || fieldKey == "" {
		s.respondError(w, http.StatusBadRequest, "name and field are required")
		return
	}
	var req struct {
		Key   string `json:"key"`
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		s.respondError(w, http.StatusBadRequest, "value is required")
		return
	}
	if bodyKey := strings.TrimSpace(req.Key); bodyKey != "" && bodyKey != fieldKey {
		s.respondError(w, http.StatusBadRequest, "body key must match path field")
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
	s.handleAgentSaveSecretFields(w, r)
}

func (s *Server) handleVaultKeyFieldDelete(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("agent", r.PathValue("vault"))
	s.handleAgentDeleteSecretField(w, r)
}

func (s *Server) handleVaultKeyActivate(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, http.StatusNotImplemented, "key activation must be performed by the local vault lifecycle boundary and mirrored back via tracked ref sync")
}

type ioNopCloser struct {
	*bytes.Reader
}

func (n ioNopCloser) Close() error { return nil }
