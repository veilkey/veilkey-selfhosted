package api

import (
	"encoding/json"
	"net/http"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

type installStatePayload struct {
	SessionID       string   `json:"session_id"`
	Version         int      `json:"version"`
	Language        string   `json:"language"`
	Quickstart      bool     `json:"quickstart"`
	Flow            string   `json:"flow"`
	DeploymentMode  string   `json:"deployment_mode"`
	InstallScope    string   `json:"install_scope"`
	BootstrapMode   string   `json:"bootstrap_mode"`
	MailTransport   string   `json:"mail_transport"`
	PlannedStages   []string `json:"planned_stages"`
	CompletedStages []string `json:"completed_stages"`
	LastStage       string   `json:"last_stage"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type installStatePatchRequest struct {
	SessionID       string    `json:"session_id"`
	Version         *int      `json:"version"`
	Language        *string   `json:"language"`
	Quickstart      *bool     `json:"quickstart"`
	Flow            *string   `json:"flow"`
	DeploymentMode  *string   `json:"deployment_mode"`
	InstallScope    *string   `json:"install_scope"`
	BootstrapMode   *string   `json:"bootstrap_mode"`
	MailTransport   *string   `json:"mail_transport"`
	PlannedStages   *[]string `json:"planned_stages"`
	CompletedStages *[]string `json:"completed_stages"`
	LastStage       *string   `json:"last_stage"`
}

func encodeStringList(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeStringList(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

func installStateToPayload(session *db.InstallSession) installStatePayload {
	return installStatePayload{
		SessionID:       session.SessionID,
		Version:         session.Version,
		Language:        session.Language,
		Quickstart:      session.Quickstart,
		Flow:            session.Flow,
		DeploymentMode:  session.DeploymentMode,
		InstallScope:    session.InstallScope,
		BootstrapMode:   session.BootstrapMode,
		MailTransport:   session.MailTransport,
		PlannedStages:   decodeStringList(session.PlannedStagesJSON),
		CompletedStages: decodeStringList(session.CompletedStagesJSON),
		LastStage:       session.LastStage,
		CreatedAt:       session.CreatedAt.UTC().Format(http.TimeFormat),
		UpdatedAt:       session.UpdatedAt.UTC().Format(http.TimeFormat),
	}
}

func installStateFromPayload(req installStatePayload) *db.InstallSession {
	return &db.InstallSession{
		SessionID:           req.SessionID,
		Version:             req.Version,
		Language:            req.Language,
		Quickstart:          req.Quickstart,
		Flow:                req.Flow,
		DeploymentMode:      req.DeploymentMode,
		InstallScope:        req.InstallScope,
		BootstrapMode:       req.BootstrapMode,
		MailTransport:       req.MailTransport,
		PlannedStagesJSON:   encodeStringList(req.PlannedStages),
		CompletedStagesJSON: encodeStringList(req.CompletedStages),
		LastStage:           req.LastStage,
	}
}

func (s *Server) handleCreateInstallSession(w http.ResponseWriter, r *http.Request) {
	var req installStatePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" {
		req.SessionID = crypto.GenerateUUID()
	}
	session := installStateFromPayload(req)
	if err := s.db.SaveInstallSession(session); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.db.GetInstallSession(req.SessionID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to reload install session")
		return
	}
	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"session": installStateToPayload(saved),
	})
}

func (s *Server) handleGetInstallState(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	var (
		session *db.InstallSession
		err     error
	)
	if sessionID != "" {
		session, err = s.db.GetInstallSession(sessionID)
	} else {
		session, err = s.db.GetLatestInstallSession()
	}
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"exists": false,
		})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"exists":  true,
		"session": installStateToPayload(session),
	})
}

func (s *Server) handlePatchInstallState(w http.ResponseWriter, r *http.Request) {
	var req installStatePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SessionID == "" {
		s.respondError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	session, err := s.db.GetInstallSession(req.SessionID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if req.Version != nil {
		session.Version = *req.Version
	}
	if req.Language != nil {
		session.Language = *req.Language
	}
	if req.Quickstart != nil {
		session.Quickstart = *req.Quickstart
	}
	if req.Flow != nil {
		session.Flow = *req.Flow
	}
	if req.DeploymentMode != nil {
		session.DeploymentMode = *req.DeploymentMode
	}
	if req.InstallScope != nil {
		session.InstallScope = *req.InstallScope
	}
	if req.BootstrapMode != nil {
		session.BootstrapMode = *req.BootstrapMode
	}
	if req.MailTransport != nil {
		session.MailTransport = *req.MailTransport
	}
	if req.PlannedStages != nil {
		session.PlannedStagesJSON = encodeStringList(*req.PlannedStages)
	}
	if req.CompletedStages != nil {
		session.CompletedStagesJSON = encodeStringList(*req.CompletedStages)
	}
	if req.LastStage != nil {
		session.LastStage = *req.LastStage
	}
	if err := s.db.SaveInstallSession(session); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"session": installStateToPayload(session),
	})
}
