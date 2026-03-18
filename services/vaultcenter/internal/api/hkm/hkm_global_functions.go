package hkm

import (
	"net/http"
	"veilkey-vaultcenter/internal/httputil"
	"strings"
	"veilkey-vaultcenter/internal/db"
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
		if err := h.deps.DB().SaveGlobalFunction(&req); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
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
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		if err := h.deps.DB().DeleteGlobalFunction(name); err != nil {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
