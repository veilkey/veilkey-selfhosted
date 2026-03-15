package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"veilkey-localvault/internal/db"
)

func (s *Server) CleanupExpiredTestFunctions(now time.Time) (int, error) {
	return s.db.CleanupExpiredTestFunctions(now)
}

type globalFunctionEnvelope struct {
	Functions []db.Function `json:"functions"`
}

func (s *Server) SyncGlobalFunctions(endpoint string) (int, int, error) {
	resp, err := http.Get(endpoint)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, 0, fmt.Errorf("global function sync failed %d: %s", resp.StatusCode, string(body))
	}

	var payload globalFunctionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, 0, err
	}

	vaultHash := ""
	if s.identity != nil {
		vaultHash = s.identity.VaultHash
	}

	seen := make(map[string]bool, len(payload.Functions))
	upserted := 0
	for _, fn := range payload.Functions {
		seen[fn.Name] = true
		fn.Scope = "GLOBAL"
		fn.VaultHash = vaultHash
		if err := s.db.SaveFunction(&fn); err != nil {
			return upserted, 0, err
		}
		upserted++
	}

	functions, err := s.db.ListFunctions()
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
		if err := s.db.DeleteFunction(fn.Name); err != nil {
			return upserted, deleted, err
		}
		deleted++
	}

	return upserted, deleted, nil
}

func (s *Server) handleFunctions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		functions, err := s.db.ListFunctionsByScope(scope)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{
			"functions": functions,
			"count":     len(functions),
		})
	case http.MethodPost:
		var req db.Function
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if req.Name == "" {
			s.respondError(w, http.StatusBadRequest, "function name is required")
			return
		}
		if strings.EqualFold(req.Scope, "GLOBAL") {
			s.respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by KeyCenter sync only")
			return
		}
		if err := s.db.SaveFunction(&req); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, req)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFunction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "function name is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		fn, err := s.db.GetFunction(name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, fn)
	case http.MethodDelete:
		fn, err := s.db.GetFunction(name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.EqualFold(fn.Scope, "GLOBAL") {
			s.respondError(w, http.StatusBadRequest, "GLOBAL functions are managed by KeyCenter sync only")
			return
		}
		if err := s.db.DeleteFunction(name); err != nil {
			s.respondError(w, http.StatusNotFound, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]any{"deleted": name})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
