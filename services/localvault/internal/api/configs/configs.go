package configs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/veilkey/veilkey-go-package/httputil"
)

func (h *Handler) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.deps.DB().ListConfigs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list configs")
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
		result = append(result, configResp{
			Key:    c.Key,
			Value:  c.Value,
			Ref:    veRef(c.Scope, c.Key),
			Scope:  string(c.Scope),
			Status: string(c.Status),
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"configs": result,
		"count":   len(result),
	})
}

func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !isValidResourceName(key) {
		respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}

	config, err := h.deps.DB().GetConfig(key)
	if err != nil {
		respondError(w, http.StatusNotFound, "config not found")
		return
	}
	if config.Status == refStatusBlock {
		respondError(w, http.StatusLocked, "ref is blocked: "+veRef(config.Scope, config.Key))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":    config.Key,
		"value":  config.Value,
		"ref":    veRef(config.Scope, config.Key),
		"scope":  config.Scope,
		"status": config.Status,
	})
}

func (h *Handler) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string  `json:"key"`
		Value *string `json:"value"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !isValidResourceName(req.Key) {
		respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}
	if req.Value == nil {
		respondError(w, http.StatusBadRequest, "value is required (use DELETE to remove a config)")
		return
	}

	if err := h.deps.DB().SaveConfig(req.Key, *req.Value); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	h.deps.DB().BumpContentVersion()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":    req.Key,
		"value":  *req.Value,
		"ref":    veRef(refScopeLocal, req.Key),
		"scope":  refScopeLocal,
		"status": refStatusActive,
		"action": "saved",
	})
}

func (h *Handler) handleSaveConfigsBulk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs map[string]string `json:"configs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Configs) == 0 {
		respondError(w, http.StatusBadRequest, "configs map is required")
		return
	}
	if len(req.Configs) > httputil.MaxBulkItems {
		respondError(w, http.StatusBadRequest, "too many configs (max 200)")
		return
	}

	for k := range req.Configs {
		if !isValidResourceName(k) {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid key: %s (must match [A-Z_][A-Z0-9_]*)", k))
			return
		}
	}

	if err := h.deps.DB().SaveConfigs(req.Configs); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save configs")
		return
	}
	h.deps.DB().BumpContentVersion()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"saved": len(req.Configs),
	})
}

func (h *Handler) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !isValidResourceName(key) {
		respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}

	if err := h.deps.DB().DeleteConfig(key); err != nil {
		respondError(w, http.StatusNotFound, "config not found")
		return
	}
	h.deps.DB().BumpContentVersion()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": key,
	})
}
