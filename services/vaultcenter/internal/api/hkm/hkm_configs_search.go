package hkm

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type configSearchResult struct {
	Vault            string `json:"vault"`
	VaultRuntimeHash string `json:"vault_runtime_hash"`
	AgentHash        string `json:"agent_hash"`
	Key              string `json:"key"`
	Value            string `json:"value"`
	Scope            string `json:"scope"`
	Status           string `json:"status"`
}

func (h *Handler) handleConfigsSearch(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []configSearchResult

	for i := range agents {
		agent := &agents[i]
		if agent.AgentHash == "" {
			continue
		}
		ai := agentToInfo(agent)
		wg.Add(1)
		go func(ai *agentInfo) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ai.URL()+"/api/configs/"+key, nil)
			if err != nil {
				return
			}
			resp, err := h.deps.HTTPClient().Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			var data struct {
				Key    string `json:"key"`
				Value  string `json:"value"`
				Scope  string `json:"scope"`
				Status string `json:"status"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				return
			}
			scope, status, normalizeErr := normalizeScopeStatus(refFamilyVE, data.Scope, data.Status, refScopeLocal)
			if normalizeErr != nil {
				return
			}
			mu.Lock()
			results = append(results, configSearchResult{
				Vault:            ai.Label,
				VaultRuntimeHash: ai.AgentHash,
				AgentHash:        ai.AgentHash,
				Key:              data.Key,
				Value:            data.Value,
				Scope:            scope,
				Status:           status,
			})
			mu.Unlock()
		}(ai)
	}
	wg.Wait()

	valueSet := make(map[string]int)
	scopeSet := make(map[string]int)
	for _, r := range results {
		valueSet[r.Value]++
		scopeSet[r.Scope]++
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":           key,
		"matches":       results,
		"match_count":   len(results),
		"unique_values": len(valueSet),
		"value_summary": valueSet,
		"scope_summary": scopeSet,
	})
}
