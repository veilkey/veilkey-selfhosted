package api

import (
	"encoding/json"
	"log"
	"net/http"

	"veilkey-localvault/internal/db"
)

type lifecycleResponse struct {
	Ciphertext string `json:"ciphertext"`
	Status     string `json:"status"`
	Changed    bool   `json:"changed"`
	SyncStatus string `json:"sync_status,omitempty"`
	SyncTarget string `json:"sync_target,omitempty"`
	SyncError  string `json:"sync_error,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

func (s *Server) handleReencrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ciphertext == "" {
		s.respondError(w, http.StatusBadRequest, "ciphertext is required")
		return
	}
	parsed, err := ParseScopedVKRef(req.Ciphertext)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}
	if _, err := s.db.GetSecretByRef(parsed.ID); err != nil {
		s.respondError(w, http.StatusNotFound, "ref not found: "+parsed.ID)
		return
	}
	canonical := parsed.CanonicalString()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"ciphertext": canonical,
		"changed":    parsed.Raw != canonical,
	})
}

func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ciphertext string `json:"ciphertext"`
		Scope      string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ciphertext == "" {
		s.respondError(w, http.StatusBadRequest, "ciphertext is required")
		return
	}

	parsed, err := ParseScopedRef(req.Ciphertext)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}
	if parsed.Scope != RefScopeTemp {
		s.respondError(w, http.StatusBadRequest, "ciphertext must use TEMP scope")
		return
	}

	targetScope, err := ParseActivationScope(req.Scope)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}
	switch parsed.Family {
	case RefFamilyVK:
		secret, err := s.db.GetSecretByRef(parsed.ID)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "ref not found: "+parsed.ID)
			return
		}
		if secret.Status == db.RefStatusBlock {
			s.respondError(w, http.StatusLocked, "ref is blocked: "+parsed.CanonicalString())
			return
		}
		if err := s.db.UpdateSecretLifecycle(parsed.ID, targetScope, db.RefStatusActive); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
		activated := ParsedRef{
			Family: parsed.Family,
			Scope:  targetScope,
			ID:     parsed.ID,
		}
		s.respondLifecycleJSON(w, activated.CanonicalString(), db.RefStatusActive, true, s.syncTrackedRefWithVaultcenter(activated.CanonicalString(), parsed.CanonicalString(), secret.Version, db.RefStatusActive))
		return
	case RefFamilyVE:
		config, err := s.db.GetConfig(parsed.ID)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "ref not found: "+parsed.ID)
			return
		}
		if config.Status == db.RefStatusBlock {
			s.respondError(w, http.StatusLocked, "ref is blocked: "+parsed.CanonicalString())
			return
		}
		if err := s.db.UpdateConfigLifecycle(parsed.ID, targetScope, db.RefStatusActive); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
		activated := ParsedRef{
			Family: parsed.Family,
			Scope:  targetScope,
			ID:     parsed.ID,
		}
		s.respondLifecycleJSON(w, activated.CanonicalString(), db.RefStatusActive, true, s.syncTrackedRefWithVaultcenter(activated.CanonicalString(), parsed.CanonicalString(), 0, db.RefStatusActive))
		return
	default:
		s.respondError(w, http.StatusBadRequest, "family must be VK or VE")
		return
	}
}

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	s.handleStatusTransition(w, r, db.RefStatusArchive)
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	s.handleStatusTransition(w, r, db.RefStatusBlock)
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request) {
	s.handleStatusTransition(w, r, db.RefStatusRevoke)
}

func (s *Server) handleStatusTransition(w http.ResponseWriter, r *http.Request, status db.RefStatus) {
	var req struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Ciphertext == "" {
		s.respondError(w, http.StatusBadRequest, "ciphertext is required")
		return
	}

	parsed, err := ParseScopedRef(req.Ciphertext)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid ref format")
		return
	}
	if parsed.Scope == RefScopeTemp {
		s.respondError(w, http.StatusBadRequest, "ciphertext must use LOCAL or EXTERNAL scope")
		return
	}
	switch parsed.Family {
	case RefFamilyVK:
		secret, err := s.db.GetSecretByRef(parsed.ID)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "ref not found: "+parsed.ID)
			return
		}
		if secret.Status == db.RefStatusBlock {
			s.respondError(w, http.StatusLocked, "ref is blocked: "+parsed.CanonicalString())
			return
		}
		scope := secret.Scope
		if scope == "" {
			scope = parsed.Scope
		}
		if err := s.db.UpdateSecretLifecycle(parsed.ID, scope, status); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
		s.respondLifecycleJSON(w, parsed.CanonicalString(), status, false, s.syncTrackedRefWithVaultcenter(parsed.CanonicalString(), "", secret.Version, status))
		return
	case RefFamilyVE:
		config, err := s.db.GetConfig(parsed.ID)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "ref not found: "+parsed.ID)
			return
		}
		if config.Status == db.RefStatusBlock {
			s.respondError(w, http.StatusLocked, "ref is blocked: "+parsed.CanonicalString())
			return
		}
		if err := s.db.UpdateConfigLifecycle(parsed.ID, "", status); err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
		s.respondLifecycleJSON(w, parsed.CanonicalString(), status, false, s.syncTrackedRefWithVaultcenter(parsed.CanonicalString(), "", 0, status))
		return
	default:
		s.respondError(w, http.StatusBadRequest, "family must be VK or VE")
		return
	}
}

func (s *Server) respondLifecycleJSON(w http.ResponseWriter, ciphertext string, status db.RefStatus, changed bool, sync trackedRefSyncResult) {
	resp := lifecycleResponse{
		Ciphertext: ciphertext,
		Status:     string(status),
		Changed:    changed,
		SyncStatus: sync.Status,
		SyncTarget: sync.URL,
		SyncError:  sync.Error,
	}
	if sync.Status == "degraded" {
		resp.Warning = "lifecycle updated but tracked-ref sync degraded"
		log.Printf("tracked-ref sync degraded: target=%s source=%s error=%s", sync.URL, sync.Source, sync.Error)
		for _, warning := range sync.Warnings {
			log.Printf("tracked-ref sync warning: %s", warning)
		}
	}
	if sync.Status == "skipped" && sync.URL == "" {
		resp.Warning = "tracked-ref sync skipped: vaultcenter target not resolved"
	}
	s.respondJSON(w, http.StatusOK, resp)
}
