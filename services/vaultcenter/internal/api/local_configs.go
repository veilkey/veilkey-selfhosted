package api

import (
	"fmt"
	"net/http"
)

func localSupportedFeatures() []string {
	return []string{
		"status",
		"node_info",
		"secrets",
		"configs",
		"resolve",
	}
}

func (s *Server) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := s.db.ListConfigs()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list configs")
		return
	}

	type configResp struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Ref    string `json:"ref"`
		Scope  string `json:"scope"`
		Status string `json:"status"`
	}

	result := make([]configResp, 0, len(configs))
	for _, c := range configs {
		scope, status, err := normalizeScopeStatus("VE", c.Scope, c.Status, "LOCAL")
		if err != nil {
			continue
		}
		ref := fmt.Sprintf("VE:%s:%s", scope, c.Key)
		result = append(result, configResp{
			Key:    c.Key,
			Value:  c.Value,
			Ref:    ref,
			Scope:  scope,
			Status: status,
		})
		_ = s.upsertTrackedRef(ref, 1, status, "")
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"configs": result,
		"count":   len(result),
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		s.respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	config, err := s.db.GetConfig(key)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	scope, status, err := normalizeScopeStatus("VE", config.Scope, config.Status, "LOCAL")
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "invalid config state")
		return
	}
	ref := fmt.Sprintf("VE:%s:%s", scope, config.Key)
	_ = s.upsertTrackedRef(ref, 1, status, "")

	s.respondJSON(w, http.StatusOK, map[string]any{
		"key":    config.Key,
		"value":  config.Value,
		"ref":    ref,
		"scope":  scope,
		"status": status,
	})
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string  `json:"key"`
		Value *string `json:"value"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		s.respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !isValidResourceName(req.Key) {
		s.respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}
	if req.Value == nil {
		s.respondError(w, http.StatusBadRequest, "value is required (use DELETE to remove a config)")
		return
	}

	before := map[string]any{}
	if existing, err := s.db.GetConfig(req.Key); err == nil && existing != nil {
		before = map[string]any{
			"key":    existing.Key,
			"value":  existing.Value,
			"scope":  existing.Scope,
			"status": existing.Status,
		}
	}

	if err := s.db.SaveConfig(req.Key, *req.Value); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	ref := "VE:LOCAL:" + req.Key
	_ = s.upsertTrackedRef(ref, 1, "active", "")
	s.saveAuditEvent(
		"config",
		ref,
		"save",
		"api",
		actorIDForRequest(r),
		"",
		"local_config_save",
		before,
		map[string]any{
			"key":    req.Key,
			"value":  *req.Value,
			"ref":    ref,
			"scope":  "LOCAL",
			"status": "active",
		},
	)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"key":    req.Key,
		"value":  *req.Value,
		"ref":    ref,
		"scope":  "LOCAL",
		"status": "active",
		"action": "saved",
	})
}

func (s *Server) handleSaveConfigsBulk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs map[string]string `json:"configs"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Configs) == 0 {
		s.respondError(w, http.StatusBadRequest, "configs map is required")
		return
	}
	for key := range req.Configs {
		if !isValidResourceName(key) {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid key: %s (must match [A-Z_][A-Z0-9_]*)", key))
			return
		}
	}

	if err := s.db.SaveConfigs(req.Configs); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save configs: "+err.Error())
		return
	}

	for key, value := range req.Configs {
		ref := "VE:LOCAL:" + key
		_ = s.upsertTrackedRef(ref, 1, "active", "")
		s.saveAuditEvent(
			"config",
			ref,
			"save",
			"api",
			actorIDForRequest(r),
			"",
			"local_config_bulk_save",
			nil,
			map[string]any{
				"key":    key,
				"value":  value,
				"ref":    ref,
				"scope":  "LOCAL",
				"status": "active",
			},
		)
	}

	s.respondJSON(w, http.StatusOK, map[string]any{
		"saved": len(req.Configs),
	})
}

func (s *Server) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		s.respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	before := map[string]any{}
	if existing, err := s.db.GetConfig(key); err == nil && existing != nil {
		before = map[string]any{
			"key":    existing.Key,
			"value":  existing.Value,
			"scope":  existing.Scope,
			"status": existing.Status,
		}
	}

	if err := s.db.DeleteConfig(key); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	ref := "VE:LOCAL:" + key
	_ = s.deleteTrackedRef(ref)
	s.saveAuditEvent(
		"config",
		ref,
		"delete",
		"api",
		actorIDForRequest(r),
		"",
		"local_config_delete",
		before,
		map[string]any{
			"deleted": key,
			"ref":     ref,
		},
	)

	s.respondJSON(w, http.StatusOK, map[string]any{
		"deleted": key,
	})
}
