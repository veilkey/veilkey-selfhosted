package hkm

import (
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	chain "github.com/veilkey/veilkey-chain"
)

func (h *Handler) handleGlobalFunctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		functions, err := h.deps.DB().ListGlobalFunctions()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list global functions")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"functions": functions,
			"count":     len(functions),
		})
	case http.MethodPost:
		var req db.GlobalFunction
		if err := httputil.DecodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			respondError(w, http.StatusBadRequest, "function name is required")
			return
		}
		if _, err := h.deps.SubmitTx(r.Context(), chain.TxSaveGlobalFunction, chain.SaveGlobalFunctionPayload{
			Name:         req.Name,
			FunctionHash: req.FunctionHash,
			Category:     req.Category,
			Command:      req.Command,
			VarsJSON:     req.VarsJSON,
		}); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request")
			return
		}
		respondJSON(w, http.StatusOK, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleGlobalFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		fn, err := h.deps.DB().GetGlobalFunction(name)
		if err != nil {
			respondError(w, http.StatusNotFound, "not found")
			return
		}
		respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		if _, err := h.deps.SubmitTx(r.Context(), chain.TxDeleteGlobalFunction, chain.DeleteGlobalFunctionPayload{Name: name}); err != nil {
			respondError(w, http.StatusNotFound, "not found")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
