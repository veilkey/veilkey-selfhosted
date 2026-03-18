package api

import (
	"net/http"
	"strings"
	"veilkey-vaultcenter/internal/db"
)

func (s *Server) handleGlobalFunctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		functions, err := s.db.ListGlobalFunctions()
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to list global functions")
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{
			"functions": functions,
			"count":     len(functions),
		})
	case http.MethodPost:
		var req db.GlobalFunction
		if err := decodeJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			s.respondError(w, http.StatusBadRequest, "function name is required")
			return
		}
		if err := s.db.SaveGlobalFunction(&req); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGlobalFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		fn, err := s.db.GetGlobalFunction(name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		if err := s.db.DeleteGlobalFunction(name); err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
