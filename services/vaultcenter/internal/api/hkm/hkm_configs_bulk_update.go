package hkm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"sync"
	"time"
)

func (h *Handler) handleConfigsBulkUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key      string `json:"key"`
		NewKey   string `json:"new_key,omitempty"`
		OldValue string `json:"old_value"`
		NewValue string `json:"new_value"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.NewValue == "" {
		respondError(w, http.StatusBadRequest, "key and new_value are required")
		return
	}

	// Safety: cannot change both key name AND value simultaneously
	keyChanging := req.NewKey != "" && req.NewKey != req.Key
	if keyChanging {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "cannot change both key name and value in a single bulk operation",
			"key":     req.Key,
			"new_key": req.NewKey,
			"hint":    "change key name first (same value), then change value (same key), or vice versa",
		})
		return
	}

	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// First pass: find which agents have this key and their current values
	type agentConfig struct {
		ai     *agentInfo
		value  string
		scope  string
		status string
	}
	var targets []agentConfig
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range agents {
		agent := &agents[i]
		if agent.AgentHash == "" {
			continue
		}
		ai := agentToInfo(agent)
		wg.Add(1)
		go func(ai *agentInfo) {
			defer wg.Done()
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, joinPath(ai.URL(), agentPathConfigs, req.Key), nil)
			if err != nil {
				return
			}
			resp, err := h.deps.HTTPClient().Do(httpReq)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				return
			}
			defer resp.Body.Close()
			var data struct {
				Value  string `json:"value"`
				Scope  string `json:"scope"`
				Status string `json:"status"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				normScope, normStatus, normalizeErr := normalizeScopeStatus(refFamilyVE, refScope(data.Scope), refStatus(data.Status), refScopeLocal)
				if normalizeErr != nil {
					return
				}
				mu.Lock()
				targets = append(targets, agentConfig{ai: ai, value: data.Value, scope: string(normScope), status: string(normStatus)})
				mu.Unlock()
			}
		}(ai)
	}
	wg.Wait()

	if len(targets) == 0 {
		respondError(w, http.StatusNotFound, fmt.Sprintf("key %s not found on any agent", req.Key))
		return
	}

	// Check unique values
	valueSet := make(map[string]int)
	for _, t := range targets {
		valueSet[t.value]++
	}

	// If multiple unique values and no old_value specified, require it
	if len(valueSet) > 1 && req.OldValue == "" {
		respondJSON(w, http.StatusConflict, map[string]interface{}{
			"error":         "multiple values exist for this key, old_value is required",
			"key":           req.Key,
			"unique_values": len(valueSet),
			"value_summary": valueSet,
		})
		return
	}

	// Determine which agents to update
	var toUpdate []agentConfig
	for _, t := range targets {
		if req.OldValue != "" && t.value != req.OldValue {
			continue
		}
		toUpdate = append(toUpdate, t)
	}

	if len(toUpdate) == 0 {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"key":     req.Key,
			"updated": 0,
			"skipped": len(targets),
		})
		return
	}

	// Apply to all targets, collect results
	type applyResult struct {
		ac  agentConfig
		err error
	}
	results := make([]applyResult, len(toUpdate))
	var updateWg sync.WaitGroup
	for i, t := range toUpdate {
		updateWg.Add(1)
		go func(idx int, ac agentConfig) {
			defer updateWg.Done()
			body, marshalErr := json.Marshal(map[string]string{
				"key":    req.Key,
				"value":  req.NewValue,
				"scope":  ac.scope,
				"status": ac.status,
			})
			if marshalErr != nil {
				results[idx] = applyResult{ac: ac, err: marshalErr}
				return
			}
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ac.ai.URL()+agentPathConfigs, bytes.NewReader(body))
			if err != nil {
				results[idx] = applyResult{ac: ac, err: err}
				return
			}
			httpReq.Header.Set("Content-Type", httputil.ContentTypeJSON)
			resp, err := h.deps.HTTPClient().Do(httpReq)
			if err != nil {
				results[idx] = applyResult{ac: ac, err: err}
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				results[idx] = applyResult{ac: ac, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
				return
			}
			results[idx] = applyResult{ac: ac}
		}(i, t)
	}
	updateWg.Wait()

	// Check for failures
	var failed []string
	var succeeded []agentConfig
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", r.ac.ai.Label, r.err))
		} else {
			succeeded = append(succeeded, r.ac)
		}
	}

	// All-or-nothing: if any failed, rollback succeeded ones
	if len(failed) > 0 {
		var rollbackWg sync.WaitGroup
		for _, ac := range succeeded {
			rollbackWg.Add(1)
			go func(ac agentConfig) {
				defer rollbackWg.Done()
				body, marshalErr := json.Marshal(map[string]string{
					"key":    req.Key,
					"value":  ac.value,
					"scope":  ac.scope,
					"status": ac.status,
				})
				if marshalErr != nil {
					return
				}
				httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, ac.ai.URL()+agentPathConfigs, bytes.NewReader(body))
				if httpReq != nil {
					httpReq.Header.Set("Content-Type", httputil.ContentTypeJSON)
					resp, err := h.deps.HTTPClient().Do(httpReq)
					if err == nil {
						resp.Body.Close()
					}
				}
			}(ac)
		}
		rollbackWg.Wait()

		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":       "bulk-update failed, all changes rolled back",
			"failed":      failed,
			"rolled_back": len(succeeded),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":     req.Key,
		"updated": len(succeeded),
		"skipped": len(targets) - len(toUpdate),
	})
}
