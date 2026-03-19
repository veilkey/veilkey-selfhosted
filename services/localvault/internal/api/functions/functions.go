package functions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"veilkey-localvault/internal/db"
)

type globalFunctionEnvelope struct {
	Functions []db.Function `json:"functions"`
}

// SyncGlobalFunctions fetches global functions from the given endpoint and
// upserts them locally, deleting any GLOBAL-scoped functions not in the response.
// Called by the cron runner via forwarding method on *api.Server.
func (h *Handler) SyncGlobalFunctions(endpoint string) (int, int, error) {
	resp, err := h.deps.HTTPClient().Get(endpoint)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, 0, fmt.Errorf("global function sync failed %d: (failed to read body: %v)", resp.StatusCode, err)
		}
		return 0, 0, fmt.Errorf("global function sync failed %d: %s", resp.StatusCode, string(body))
	}

	var payload globalFunctionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, 0, err
	}

	vaultHash := h.deps.VaultHash()

	seen := make(map[string]bool, len(payload.Functions))
	upserted := 0
	for _, fn := range payload.Functions {
		seen[fn.Name] = true
		fn.Scope = "GLOBAL"
		fn.VaultHash = vaultHash
		if err := h.deps.DB().SaveFunction(&fn); err != nil {
			return upserted, 0, err
		}
		upserted++
	}

	functions, err := h.deps.DB().ListFunctions()
	if err != nil {
		return upserted, 0, err
	}
	deleted := 0
	for _, fn := range functions {
		if fn.Scope != "GLOBAL" {
			continue
		}
		if seen[fn.Name] {
			continue
		}
		if err := h.deps.DB().DeleteFunction(fn.Name); err != nil {
			return upserted, deleted, err
		}
		deleted++
	}

	return upserted, deleted, nil
}

func (h *Handler) handleFunctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		functions, err := h.deps.DB().ListFunctionsByScope(scope)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid scope")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"functions": functions,
			"count":     len(functions),
		})
	case http.MethodPost:
		var req db.Function
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Name == "" {
			respondError(w, http.StatusBadRequest, "function name is required")
			return
		}
		if strings.EqualFold(req.Scope, "GLOBAL") {
			respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by VaultCenter sync only")
			return
		}
		if err := h.deps.DB().SaveFunction(&req); err != nil {
			respondError(w, http.StatusBadRequest, "failed to save function")
			return
		}
		respondJSON(w, http.StatusOK, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		fn, err := h.deps.DB().GetFunction(name)
		if err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		fn, err := h.deps.DB().GetFunction(name)
		if err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		if strings.EqualFold(fn.Scope, "GLOBAL") {
			respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by VaultCenter sync only")
			return
		}
		if err := h.deps.DB().DeleteFunction(name); err != nil {
			respondError(w, http.StatusNotFound, "function not found")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
