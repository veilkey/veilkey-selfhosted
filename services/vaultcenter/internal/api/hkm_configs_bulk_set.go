package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type bulkSetCheck struct {
	ai        *agentInfo
	oldValue  string
	oldScope  string
	oldStatus string
	found     bool
}

// POST /api/configs/bulk-set
// Sets a key=value on ALL agents (or creates if not exists).
// All-or-nothing: if any agent fails, the entire operation is rolled back.
// Request: {"key": "VEILKEY_VAULTCENTER_URL", "value": "http://your-hub:<port>"}
// Optional: {"key": "...", "value": "...", "overwrite": false}  — skip agents that already have the key
func (s *Server) handleConfigsBulkSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		Scope     string `json:"scope"`
		Status    string `json:"status"`
		Overwrite *bool  `json:"overwrite,omitempty"` // default true
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" || req.Value == "" {
		s.respondError(w, http.StatusBadRequest, "key and value are required")
		return
	}
	if !isValidResourceName(req.Key) {
		s.respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}
	scope, status, err := normalizeScopeStatus("VE", req.Scope, req.Status, "LOCAL")
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	overwrite := true
	if req.Overwrite != nil {
		overwrite = *req.Overwrite
	}

	agents, err := s.db.ListAgents()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Pre-flight: check current values across all agents
	var checks []bulkSetCheck
	var checkMu sync.Mutex
	var checkWg sync.WaitGroup

	for i := range agents {
		agent := &agents[i]
		if agent.AgentHash == "" {
			continue
		}
		ai := agentToInfo(agent)
		checkWg.Add(1)
		go func(ai *agentInfo) {
			defer checkWg.Done()
			httpReq, err := http.NewRequestWithContext(ctx, "GET", ai.URL()+"/api/configs/"+req.Key, nil)
			if err != nil {
				checkMu.Lock()
				checks = append(checks, bulkSetCheck{ai: ai, found: false})
				checkMu.Unlock()
				return
			}
			resp, err := s.httpClient.Do(httpReq)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				checkMu.Lock()
				checks = append(checks, bulkSetCheck{ai: ai, found: false})
				checkMu.Unlock()
				return
			}
			defer resp.Body.Close()
			var data struct {
				Value  string `json:"value"`
				Scope  string `json:"scope"`
				Status string `json:"status"`
			}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				currentScope, currentStatus, normalizeErr := normalizeScopeStatus("VE", data.Scope, data.Status, "LOCAL")
				if normalizeErr != nil {
					checkMu.Lock()
					checks = append(checks, bulkSetCheck{ai: ai, found: false})
					checkMu.Unlock()
					return
				}
				checkMu.Lock()
				checks = append(checks, bulkSetCheck{ai: ai, oldValue: data.Value, oldScope: currentScope, oldStatus: currentStatus, found: true})
				checkMu.Unlock()
			}
		}(ai)
	}
	checkWg.Wait()

	// Safety: if ALL agents already have this exact key+value, reject as no-op
	if len(checks) > 0 {
		allSame := true
		for _, c := range checks {
			if !c.found || c.oldValue != req.Value {
				allSame = false
				break
			}
		}
		if allSame {
			s.respondJSON(w, http.StatusConflict, map[string]interface{}{
				"error":       "all agents already have this exact key and value, no change needed",
				"key":         req.Key,
				"value":       req.Value,
				"agent_count": len(checks),
			})
			return
		}
	}

	// Determine targets
	var targets []*agentInfo
	for i := range checks {
		c := &checks[i]
		if !overwrite && c.found {
			continue // overwrite=false, key exists → skip
		}
		if c.found && c.oldValue == req.Value {
			continue // already has exact same value → skip
		}
		targets = append(targets, c.ai)
	}

	if len(targets) == 0 {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"key":     req.Key,
			"value":   req.Value,
			"updated": 0,
		})
		return
	}

	// Apply to all targets in parallel, collect results
	type applyResult struct {
		ai  *agentInfo
		err error
	}
	results := make([]applyResult, len(targets))
	var wg sync.WaitGroup
	for i, ai := range targets {
		wg.Add(1)
		go func(idx int, ai *agentInfo) {
			defer wg.Done()
			appliedScope := scope
			appliedStatus := status
			for _, c := range checks {
				if c.ai.Label == ai.Label && c.found {
					appliedScope = c.oldScope
					appliedStatus = c.oldStatus
					break
				}
			}
			body, marshalErr := json.Marshal(map[string]string{
				"key":    req.Key,
				"value":  req.Value,
				"scope":  appliedScope,
				"status": appliedStatus,
			})
			if marshalErr != nil {
				results[idx] = applyResult{ai: ai, err: marshalErr}
				return
			}
			httpReq, err := http.NewRequestWithContext(ctx, "POST", ai.URL()+"/api/configs", bytes.NewReader(body))
			if err != nil {
				results[idx] = applyResult{ai: ai, err: err}
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			resp, err := s.httpClient.Do(httpReq)
			if err != nil {
				results[idx] = applyResult{ai: ai, err: err}
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				results[idx] = applyResult{ai: ai, err: fmt.Errorf("HTTP %s", http.StatusText(resp.StatusCode))}
				return
			}
			results[idx] = applyResult{ai: ai}
		}(i, ai)
	}
	wg.Wait()

	// Check for failures
	var failed []string
	var succeeded []*agentInfo
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, r.ai.Label+": "+r.err.Error())
		} else {
			succeeded = append(succeeded, r.ai)
		}
	}

	// All-or-nothing: if any failed, rollback succeeded ones
	if len(failed) > 0 {
		s.rollbackBulkSet(ctx, succeeded, checks, req.Key)
		s.respondJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":       "bulk-set failed, all changes rolled back",
			"failed":      failed,
			"rolled_back": len(succeeded),
		})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":     req.Key,
		"value":   req.Value,
		"updated": len(succeeded),
	})
}

// rollbackBulkSet restores original values on agents that were successfully updated
func (s *Server) rollbackBulkSet(ctx context.Context, agents []*agentInfo, checks []bulkSetCheck, key string) {
	// Build oldValue lookup from pre-flight checks
	oldValues := make(map[string]bulkSetCheck)
	for _, c := range checks {
		oldValues[c.ai.Label] = c
	}

	var wg sync.WaitGroup
	for _, ai := range agents {
		c, ok := oldValues[ai.Label]
		if !ok {
			continue
		}

		wg.Add(1)
		if c.found {
			// Restore old value
			go func(ai *agentInfo, oldVal string, oldScope string, oldStatus string) {
				defer wg.Done()
				body, marshalErr := json.Marshal(map[string]string{
					"key":    key,
					"value":  oldVal,
					"scope":  oldScope,
					"status": oldStatus,
				})
				if marshalErr != nil {
					return
				}
				httpReq, _ := http.NewRequestWithContext(ctx, "POST", ai.URL()+"/api/configs", bytes.NewReader(body))
				if httpReq != nil {
					httpReq.Header.Set("Content-Type", "application/json")
					resp, err := s.httpClient.Do(httpReq)
					if err == nil {
						resp.Body.Close()
					}
				}
			}(ai, c.oldValue, c.oldScope, c.oldStatus)
		} else {
			// Key didn't exist before → delete it
			go func(ai *agentInfo) {
				defer wg.Done()
				httpReq, _ := http.NewRequestWithContext(ctx, "DELETE", ai.URL()+"/api/configs/"+key, nil)
				if httpReq != nil {
					resp, err := s.httpClient.Do(httpReq)
					if err == nil {
						resp.Body.Close()
					}
				}
			}(ai)
		}
	}
	wg.Wait()
}
