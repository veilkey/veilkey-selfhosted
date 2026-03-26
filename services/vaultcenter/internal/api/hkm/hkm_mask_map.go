package hkm

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"veilkey-vaultcenter/internal/db"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// handleMaskMap serves the full mask_map for veil-cli PTY masking.
// Supports long polling: if ?version=N matches current version, waits up to ?wait=Ns.
// Phase 3: returns cached data when valid; rebuilds and caches on miss.
func (h *Handler) handleMaskMap(w http.ResponseWriter, r *http.Request) {
	clientVersion, _ := strconv.ParseUint(r.URL.Query().Get("version"), 10, 64)
	waitSec, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if waitSec < 0 {
		waitSec = 0
	}
	if waitSec > 60 {
		waitSec = 60
	}

	serverVersion := h.deps.MaskMapVersion()

	// Long poll: if client is up to date, wait for changes
	if clientVersion >= serverVersion && waitSec > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(waitSec)*time.Second)
		defer cancel()
		select {
		case <-h.deps.MaskMapWait():
			serverVersion = h.deps.MaskMapVersion()
		case <-ctx.Done():
			respondJSON(w, http.StatusOK, map[string]any{
				"version": serverVersion,
				"changed": false,
				"entries": []any{},
			})
			return
		}
	}

	// Fast path: serve from cache if valid and within TTL
	if cached := h.deps.GetMaskCacheData(); cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(cached)
		return
	}

	// Cache miss — rebuild mask_map from all active agents
	entries := h.buildMaskMapEntries()
	if entries == nil {
		respondError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	resp := map[string]any{
		"version": serverVersion,
		"changed": true,
		"count":   len(entries),
		"entries": entries,
	}

	// Store in cache for subsequent requests
	if data, err := json.Marshal(resp); err == nil {
		h.deps.SetMaskCacheData(data)
	}

	respondJSON(w, http.StatusOK, resp)
}

// buildMaskMapEntries constructs the full mask-map entry list from agents,
// SSH keys, and VE configs. Returns nil on fatal error (e.g. DB failure).
func (h *Handler) buildMaskMapEntries() []maskEntry {
	agents, err := h.deps.DB().ListAgents()
	if err != nil {
		return nil
	}

	var entries []maskEntry
	for i := range agents {
		agent := &agents[i]
		if len(agent.DEK) == 0 {
			continue
		}
		agentDEK, dekErr := h.decryptAgentDEK(agent.DEK, agent.DEKNonce)
		if dekErr != nil {
			continue
		}

		// Get secret catalog for this agent
		// ListSecretCatalogFiltered uses vault_hash, but we also need vault_runtime_hash match
		allCatalog, _ := h.deps.DB().ListSecretCatalog()
		var catalog []db.SecretCatalog
		for _, sec := range allCatalog {
			if sec.VaultRuntimeHash == agent.AgentHash && sec.Status == "active" {
				catalog = append(catalog, sec)
			}
		}

		ai := agentToInfo(agent)
		for _, sec := range catalog {
			// Extract raw ref from canonical (VK:LOCAL:xxx → xxx)
			ref := sec.RefCanonical
			parts := strings.SplitN(ref, ":", 3)
			rawRef := ref
			if len(parts) == 3 {
				rawRef = parts[2]
			}

			cipher, fetchErr := h.fetchAgentCiphertext(ai, rawRef)
			if fetchErr != nil {
				continue
			}
			plaintext, decErr := crypto.Decrypt(agentDEK, cipher.Ciphertext, cipher.Nonce)
			if decErr != nil {
				continue
			}
			pt := strings.TrimRight(string(plaintext), "\r\n")
			if pt == "" {
				continue
			}
			entries = append(entries, maskEntry{
				Ref:   sec.RefCanonical,
				Value: pt,
				Vault: agent.VaultName,
			})
		}
	}

	// Add SSH keys stored directly on VaultCenter (not via agent heartbeat)
	sshRefs, _ := h.deps.DB().ListRefs()
	for _, ref := range sshRefs {
		if ref.RefScope != db.RefScopeSSH || string(ref.Status) != string(db.RefStatusActive) {
			continue
		}
		localDEK, dekErr := h.deps.GetLocalDEK()
		if dekErr != nil {
			break // DEK not available, skip all SSH refs
		}
		ct, nonce, decodeErr := crypto.DecodeCiphertext(ref.Ciphertext)
		if decodeErr != nil {
			continue
		}
		plaintext, decErr := crypto.Decrypt(localDEK, ct, nonce)
		if decErr != nil {
			continue
		}
		pt := strings.TrimRight(string(plaintext), "\r\n")
		if pt != "" {
			entries = append(entries, maskEntry{
				Ref:   ref.RefCanonical,
				Value: pt,
				Vault: "vaultcenter",
			})
		}
	}

	// Add VE (config) entries — deduplicated by value to avoid repeated tagging
	veSeenValues := make(map[string]bool)
	// Also skip values that already appear as VK secrets (avoid double masking)
	for _, e := range entries {
		veSeenValues[e.Value] = true
	}
	for i := range agents {
		agent := &agents[i]
		if agent.IP == "" {
			continue
		}
		ai := agentToInfo(agent)
		configURL := ai.URL() + "/api/configs"
		req, reqErr := http.NewRequest(http.MethodGet, configURL, nil)
		if reqErr != nil {
			continue
		}
		h.setAgentAuthHeader(req, ai)
		configResp, configErr := h.deps.HTTPClient().Do(req)
		if configErr != nil {
			continue
		}
		var configData struct {
			Configs []struct {
				Key    string `json:"key"`
				Value  string `json:"value"`
				Scope  string `json:"scope"`
				Status string `json:"status"`
			} `json:"configs"`
		}
		if err := json.NewDecoder(configResp.Body).Decode(&configData); err == nil {
			for _, cfg := range configData.Configs {
				if cfg.Value == "" || cfg.Status != "active" {
					continue
				}
				if veSeenValues[cfg.Value] {
					continue // skip duplicate values
				}
				veSeenValues[cfg.Value] = true
				veRef := "VE:" + cfg.Scope + ":" + cfg.Key
				entries = append(entries, maskEntry{
					Ref:   veRef,
					Value: cfg.Value,
					Vault: agent.VaultName,
				})
			}
		}
		configResp.Body.Close()
	}

	return entries
}

// maskEntry represents a single entry in the mask map.
type maskEntry struct {
	Ref   string `json:"ref"`
	Value string `json:"value"`
	Vault string `json:"vault"`
}
